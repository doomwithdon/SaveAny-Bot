package tdler

import (
	"context"
	"sync"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/krau/SaveAny-Bot/common/utils/dlutil"
	"github.com/krau/SaveAny-Bot/config"
	"github.com/krau/SaveAny-Bot/pkg/consts/tglimit"
	"github.com/krau/SaveAny-Bot/pkg/tfile"
)

type pooledClient struct {
	client downloader.Client
	close  telegram.CloseInvoker
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
	clientPools[base] = pooledClient{client: pooled, close: invoker}
	poolMu.Unlock()

	if exists {
		_ = old.close.Close()
	}
	return nil
}

func downloadClient(base downloader.Client) downloader.Client {
	api, ok := base.(*tg.Client)
	if !ok {
		return base
	}
	poolMu.RLock()
	pooled, ok := clientPools[api]
	poolMu.RUnlock()
	if ok {
		return pooled.client
	}
	return base
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
