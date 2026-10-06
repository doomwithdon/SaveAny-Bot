package tdler

import (
	"context"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
	"github.com/krau/SaveAny-Bot/common/utils/dlutil"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/krau/SaveAny-Bot/pkg/consts/tglimit"
	"github.com/krau/SaveAny-Bot/pkg/tfile"
)

type pooledClient struct {
	client      downloader.Client
	telegram    *telegram.Client
	connections int
	close       telegram.CloseInvoker
	dcPools     map[int]telegram.CloseInvoker
	dcClients   map[int]downloader.Client
}

var (
	poolMu      sync.RWMutex
	clientPools = make(map[*tg.Client]pooledClient)
)

// RegisterClientPool creates a real multi-connection MTProto pool for downloads
// made through base. The pool uses independent data connections to the current
// Telegram DC while keeping the same authenticated account/session.
//
// This is intentionally best-effort: callers can keep using the original
// client when pool creation fails.
func RegisterClientPool(base *tg.Client, client *telegram.Client, connections int) error {
	if base == nil || client == nil {
		return nil
	}
	if connections < 1 {
		connections = 1
	}

	invoker, err := client.Pool(int64(connections))
	if err != nil {
		return err
	}
	pooled := tg.NewClient(invoker)

	poolMu.Lock()
	old, exists := clientPools[base]
	clientPools[base] = pooledClient{client: pooled, telegram: client, connections: connections, close: invoker, dcPools: make(map[int]telegram.CloseInvoker), dcClients: make(map[int]downloader.Client)}
	poolMu.Unlock()

	if exists {
		_ = old.close.Close()
		for _, p := range old.dcPools {
			_ = p.Close()
		}
	}
	return nil
}

func downloadClient(base downloader.Client) downloader.Client {
	api, ok := base.(*tg.Client)
	if !ok {
		return base
	}
	poolMu.RLock()
	_, ok := clientPools[api]
	poolMu.RUnlock()
	if ok {
		return &migratingPoolClient{base: api}
	}
	return base
}

type migratingPoolClient struct {
	base *tg.Client
}

func (c *migratingPoolClient) pool() (pooledClient, bool) {
	poolMu.RLock()
	p, ok := clientPools[c.base]
	poolMu.RUnlock()
	return p, ok
}

func (c *migratingPoolClient) clientForDC(ctx context.Context, dc int) (downloader.Client, error) {
	poolMu.Lock()
	defer poolMu.Unlock()
	p, ok := clientPools[c.base]
	if !ok {
		return c.base, nil
	}
	if dcClient, ok := p.dcClients[dc]; ok {
		return dcClient, nil
	}
	invoker, err := p.telegram.DC(ctx, dc, int64(p.connections))
	if err != nil {
		return nil, err
	}
	dcClient := tg.NewClient(invoker)
	p.dcPools[dc] = invoker
	p.dcClients[dc] = dcClient
	clientPools[c.base] = p
	return dcClient, nil
}

func (c *migratingPoolClient) UploadGetFile(ctx context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	p, ok := c.pool()
	if !ok {
		return c.base.UploadGetFile(ctx, req)
	}
	result, err := p.client.UploadGetFile(ctx, req)
	if err == nil {
		return result, nil
	}
	rpcErr, ok := tgerr.As(err)
	if !ok || rpcErr.Type != "FILE_MIGRATE" {
		return nil, err
	}
	dcClient, dcErr := c.clientForDC(ctx, rpcErr.Argument)
	if dcErr != nil {
		return nil, dcErr
	}
	return dcClient.UploadGetFile(ctx, req)
}

func (c *migratingPoolClient) UploadGetFileHashes(ctx context.Context, req *tg.UploadGetFileHashesRequest) ([]tg.FileHash, error) {
	p, ok := c.pool()
	if !ok {
		return c.base.UploadGetFileHashes(ctx, req)
	}
	return p.client.UploadGetFileHashes(ctx, req)
}

func (c *migratingPoolClient) UploadReuploadCDNFile(ctx context.Context, req *tg.UploadReuploadCDNFileRequest) ([]tg.FileHash, error) {
	p, ok := c.pool()
	if !ok {
		return c.base.UploadReuploadCDNFile(ctx, req)
	}
	return p.client.UploadReuploadCDNFile(ctx, req)
}

func (c *migratingPoolClient) UploadGetCDNFileHashes(ctx context.Context, req *tg.UploadGetCDNFileHashesRequest) ([]tg.FileHash, error) {
	p, ok := c.pool()
	if !ok {
		return c.base.UploadGetCDNFileHashes(ctx, req)
	}
	return p.client.UploadGetCDNFileHashes(ctx, req)
}

func (c *migratingPoolClient) UploadGetWebFile(ctx context.Context, req *tg.UploadGetWebFileRequest) (*tg.UploadWebFile, error) {
	p, ok := c.pool()
	if !ok {
		return c.base.UploadGetWebFile(ctx, req)
	}
	return p.client.UploadGetWebFile(ctx, req)
}

func NewDownloader(file tfile.TGFile) *downloader.Builder {
	client := downloadClient(file.Dler())
	return downloader.NewDownloader().WithPartSize(tglimit.MaxPartSize).
		Download(eofAwareClient{Client: client, size: file.Size()}, file.Location()).
		WithThreads(dlutil.BestThreads(file.Size(), config.C().Threads))
}

// eofAwareClient answers upload.getFile requests at or past the end of the
// file with an empty chunk. gotd's downloader is size-unaware: for files
// whose size is an exact multiple of the part size it issues one final
// request at offset == size and expects an empty chunk, but Telegram rejects
// it with 400 OFFSET_INVALID and the whole download fails.
type eofAwareClient struct {
	downloader.Client
	size int64
}

func (c eofAwareClient) UploadGetFile(ctx context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	if c.size > 0 && req.Offset >= c.size {
		return &tg.UploadFile{}, nil
	}
	return c.Client.UploadGetFile(ctx, req)
}
