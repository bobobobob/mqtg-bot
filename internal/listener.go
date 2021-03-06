package internal

import (
	"fmt"
	"log"
	"mqtg-bot/internal/common"
	"mqtg-bot/internal/models"
	"mqtg-bot/internal/users/menu/button_names"
	"time"
)

func (bot *TelegramBot) StartBotListener() {
	defer bot.wg.Done()

	log.Printf("Start Telegram Bot Listener")

	for {
		select {
		case <-bot.shutdownChannel:
			log.Printf("Telegram Bot Listener received shutdown signal")
			return

		case subscriptionMessage := <-bot.subscriptionCh:
			subscriptionMessage.Subscription.UserMutex.Lock()

			subStr := fmt.Sprintf("(%v, id: %v, type: %v)", subscriptionMessage.Message.Topic(), subscriptionMessage.Subscription.ID, subscriptionMessage.Subscription.SubscriptionType)

			if subscriptionMessage.Subscription.DataType == models.IMAGE_DATA_TYPE {
				log.Printf("Received new subscription %v data: %v bytes", subStr, len(subscriptionMessage.Message.Payload()))
			} else {
				log.Printf("Received new subscription %v data: %v", subStr, string(subscriptionMessage.Message.Payload()))
			}

			subscriptionMessage.Subscription.LastValueFormattedMessage = models.FormatMessage(subscriptionMessage.Subscription, subscriptionMessage.Message.Topic(), subscriptionMessage.Message.Payload())
			subscriptionMessage.Subscription.LastValuePayload = subscriptionMessage.Message.Payload()
			bot.db.Save(subscriptionMessage.Subscription)

			// need store
			switch subscriptionMessage.Subscription.SubscriptionType {
			case models.PRINT_AND_STORE_MESSAGE_SUBSCRIPTION_TYPE,
				models.SILENT_STORE_MESSAGE_SUBSCRIPTION_TYPE:
				newData := models.SubscriptionData{
					SubscriptionID:   subscriptionMessage.Subscription.ID,
					FormattedMessage: subscriptionMessage.Subscription.LastValueFormattedMessage,
					DateTime:         time.Now(),
					DataType:         subscriptionMessage.Subscription.DataType,
					Data:             subscriptionMessage.Message.Payload(),
				}
				if subscriptionMessage.Subscription.DataType == models.IMAGE_DATA_TYPE {
					log.Printf("Store new subscription %v image data: %v bytes", subStr, len(newData.Data))
				} else {
					log.Printf("Store new subscription %v data: %v", subStr, string(newData.Data))
				}
				bot.db.Create(&newData)
			}

			// need print
			switch subscriptionMessage.Subscription.SubscriptionType {
			case models.PRINT_AND_STORE_MESSAGE_SUBSCRIPTION_TYPE,
				models.PRINT_MESSAGE_WITHOUT_STORING_SUBSCRIPTION_TYPE:
				if subscriptionMessage.Subscription.DataType == models.IMAGE_DATA_TYPE {
					bot.NewPhotoUpload(
						subscriptionMessage.Subscription.ChatID,
						subscriptionMessage.Subscription.LastValueFormattedMessage,
						subscriptionMessage.Message.Payload(),
						nil,
					)
				} else {
					bot.SendMessage(
						subscriptionMessage.Subscription.ChatID,
						subscriptionMessage.Subscription.LastValueFormattedMessage,
						nil,
					)
				}
			}
			subscriptionMessage.Subscription.UserMutex.Unlock()

		case update := <-bot.updates:
			bot.metrics.numOfIncMessagesFromTelegram.Inc()

			user := bot.usersManager.GetUserByChatIdFromUpdate(&update)
			if user == nil {
				continue
			}
			user.Lock()

			var message = update.Message
			var userAnswer *common.BotMessage

			if message != nil {
				messageData := []byte(message.Text)

				var isItPhoto bool
				var photoStr string
				if message.Photo != nil {
					photoStr = fmt.Sprintf("[%v photo]", len(message.Photo))

					photoData, err := bot.DownloadPhoto(message.Photo)
					if err != nil {
						bot.SendMessage(user.ChatID, fmt.Sprintf("Download photo error: %v", err), nil)
						continue
					}
					messageData = photoData
					isItPhoto = true
				}
				log.Printf("Telegram received message from user %v (Chat.ID %v): %v %v", message.From, message.Chat.ID, message.Text, photoStr)

				switch message.Text {
				case button_names.START:
					userAnswer = user.Start()

				case button_names.CONFIGURE_CONNECTION:
					userAnswer = user.ConfigureConnection()

				case button_names.BACK:
					messageData = user.Back()
					fallthrough

				default:
					userAnswer = user.ProcessMessage(messageData, isItPhoto)
				}

			} else if update.CallbackQuery != nil {
				message = update.CallbackQuery.Message
				userAnswer = user.ProcessCallback(update.CallbackQuery)
			}

			user.Unlock()

			bot.SendAnswer(message.Chat.ID, userAnswer)
		}
	}
}
