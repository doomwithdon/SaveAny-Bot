ile(&types.ReceivedFile{
Processing: false,
FileName: file.FileName,
ChatID: update.EffectiveChat().GetID(),
MessageID: update.EffectiveMessage.ID,
ReplyMessageID: msg.ID,
ReplyChatID: update.GetUserChat().GetID(),
}); err != nil {
logger.L.Errorf("Failed to add received file: %s", err)
if _, err := ctx.EditMessage(update.EffectiveChat().GetID(), &tg.MessagesEditMessageRequest{
Message: fmt.Sprintf("Failed to add received file: %s", err),
ID: msg.ID,
}); err != nil {
logger.L.Errorf("Failed to edit message: %s", err)
}
return dispatcher.EndGroups
}

if !user.Silent {
return ProvideSelectMessage(ctx, update, file, int(update.EffectiveChat().GetID()), update.EffectiveMessage.ID, msg.ID)
}
return HandleSilentAddTask(ctx, update, user, &types.Task{
Ctx: ctx,
Status: types.Pending,
File: file,
Storage: types.StorageType(user.DefaultStorage),
FileChatID: update.EffectiveChat().GetID(),
ReplyMessageID: msg.ID,
ReplyChatID: update.GetUserChat().GetID(),
FileMessageID: update.EffectiveMessage.ID,
})
}

func AddToQueue(ctx *ext.Context, update *ext.Update) error {
if !slice.Contain(config.Cfg.Telegram.Admins, update.CallbackQuery.UserID) {
ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
QueryID: update.CallbackQuery.QueryID,
Alert: true,
Message: "You do not have permission",
CacheTime: 5,
})
return dispatcher.EndGroups
}
args := strings.Split(string(update.CallbackQuery.Data), " ")
chatID, _ := strconv.Atoi(args[1])
messageID, _ := strconv.Atoi(args[2])
storageName := args[3]
logger.L.Tracef("Got add to queue: chatID: %d, messageID: %d, storage: %s", chatID, messageID, storageName)
record, err := dao.GetReceivedFileByChatAndMessageID(int64(chatID), messageID)
if err != nil {
logger.L.Errorf("Failed to get received file: %s", err)
ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
QueryID: update.CallbackQuery.QueryID,
Alert: true,
Message: "Query record failed",
CacheTime: 5,
})
return dispatcher.EndGroups
}
if update.CallbackQuery.MsgID != record.ReplyMessageID {
record.ReplyMessageID = update.CallbackQuery.MsgID
if err := dao.SaveReceivedFile(record); err != nil {
logger.L.Errorf("Failed to update received file: %s", err)
}
}

file, err := FileFromMessage(ctx, record.ChatID, record.MessageID, record.FileName)
if err != nil {
logger.L.Errorf("Failed to get file from message: %s", err)
ctx.AnswerCallback(&tg.MessagesSetBotCallbackAnswerRequest{
QueryID: update.CallbackQuery.QueryID,
Alert: true,
Message: fmt.Sprintf("Failed to get the file in the message: %s", err),
CacheTime: 5,
})
return dispatcher.EndGroups
}

queue.AddTask(types.Task{
Ctx: ctx,
Status: types.Pending,
File: file,
Storage: types.StorageType(storageName),
FileChatID: record.ChatID,
ReplyMessageID: record.ReplyMessageID,
FileMessageID: record.MessageID,
ReplyChatID: record.ReplyChatID,
})

entityBuilder := entity.Builder{}
var entities []tg.MessageEntityClass
text := fmt.Sprintf("Added to the task queue\nFile name: %s\nCurrent number of queued tasks: %d", record.FileName, queue.Len())
if err := styling.Perform(&entityBuilder,
styling.Plain("Added to task queue\nFile name: "),
styling.Code(record.FileName),
styling.Plain("\nCurrent number of queued tasks: "),
styling.Bold(strconv.Itoa(queue.Len())),
); err != nil {
logger.L.Errorf("Failed to build entity: %s", err)
} else {
text, entities = entityBuilder.Complete()
}

ctx.EditMessage(update.EffectiveChat().GetID(), &tg.MessagesEditMessageRequest{
Message: text,
Entities: entities,
ID: record.ReplyMessageID,
})
return dispatcher.EndGroups
}
