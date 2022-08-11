package source

import (
	"errors"
	"github.com/bcicen/jstream"
	"github.com/fatih/color"
	"github.com/iyear/searchx/pkg/keygen"
	"github.com/iyear/searchx/pkg/models"
	"github.com/iyear/searchx/pkg/storage"
	"github.com/iyear/searchx/pkg/storage/search"
	"github.com/mitchellh/mapstructure"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	keyID       = "id"
	keyType     = "type"
	keyName     = "name"
	supergroup  = "supergroup"
	channel     = "channel"
	typeMessage = "message"
)

type message struct {
	ID     int         `mapstructure:"id"`
	Type   string      `mapstructure:"type"`
	Time   string      `mapstructure:"date_unixtime"`
	FromID string      `mapstructure:"from_id"`
	From   string      `mapstructure:"from"`
	Text   interface{} `mapstructure:"text"`
}

func Start(src, searchDriver string, searchOptions map[string]string) error {
	if searchDriver == "" {
		return errors.New("search driver can not be empty")
	}

	start := time.Now()

	options := make(map[string]interface{})
	if err := mapstructure.WeakDecode(searchOptions, &options); err != nil {
		return err
	}

	_search, err := storage.NewSearch(searchDriver, options)
	if err != nil {
		return err
	}

	chatType, chatID, chatName, err := getChatInfo(src)
	if err != nil {
		return err
	}

	color.Blue("Type: %s, ID: %d, Name: %s\n", chatType, chatID, chatName)

	if err = index(src, chatID, chatName, _search); err != nil {
		return err
	}
	color.Blue("Index Succ... Time: %v", time.Since(start))

	return nil

}

func index(src string, chatID int64, chatName string, _search storage.Search) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			log.Fatalln(err)
		}
	}(f)

	d := jstream.NewDecoder(f, 2)

	batchSize := 50
	items := make([]*search.Item, 0, batchSize)

	for mv := range d.Stream() {
		msg := message{}

		if mv.ValueType != jstream.Object {
			continue
		}

		if err = mapstructure.WeakDecode(mv.Value, &msg); err != nil {
			return err
		}

		if msg.ID < 0 || msg.Type != typeMessage {
			continue
		}

		text := ""

		switch r := msg.Text.(type) {
		case string:
			text = r
		case []interface{}:
			for _, tt := range r {
				switch t := tt.(type) {
				case string:
					text += t
				case map[string]interface{}:
					text += " " + t["text"].(string) + " "
				}
			}
		}

		// user: real user, channel: anonymous user
		if !strings.HasPrefix(msg.FromID, "user") && !strings.HasPrefix(msg.FromID, "channel") {
			continue
		}

		sender := strings.TrimPrefix(msg.FromID, "user")
		sender = strings.TrimPrefix(sender, "channel")

		if text != "" {
			items = append(items, &search.Item{
				ID: keygen.SearchMsgID(chatID, msg.ID),
				Data: &models.SearchMsg{
					ID:         strconv.Itoa(msg.ID),
					Chat:       strconv.FormatInt(chatID, 10),
					ChatName:   chatName,
					Text:       strings.ReplaceAll(text, "\n", " "),
					Sender:     sender,
					SenderName: msg.From,
					Date:       msg.Time,
				},
			})
		}

		if len(items) == batchSize {
			if err = _search.Index(items); err != nil {
				return err
			}
			items = make([]*search.Item, 0, batchSize)
		}
	}

	if len(items) > 0 {
		if err = _search.Index(items); err != nil {
			return err
		}
	}

	return nil

}

// getChatInfo return chat type, chat id, chat name, error
func getChatInfo(src string) (string, int64, string, error) {
	f, err := os.Open(src)
	if err != nil {
		return "", 0, "", err
	}
	defer func(f *os.File) {
		if err = f.Close(); err != nil {
			log.Fatalln(err)
		}
	}(f)

	d := jstream.NewDecoder(f, 1).EmitKV()

	chatType, chatID, chatName := "", int64(0), ""
	// var chatType = ""
	// var chatID int64 = 0

	for mv := range d.Stream() {
		kv, ok := mv.Value.(jstream.KV)
		if !ok {
			continue
		}

		if kv.Key == keyType {
			chatType = kv.Value.(string)
			if !strings.HasSuffix(chatType, supergroup) && !strings.HasSuffix(chatType, channel) {
				return "", 0, "", errors.New("chat type should be supergroup or channel")
			}
		}

		if kv.Key == keyID {
			chatID = -int64(kv.Value.(float64)) - 1e12
		}

		if kv.Key == keyName {
			chatName = kv.Value.(string)
		}

		if chatType != "" && chatID != 0 {
			break
		}
	}

	if chatType == "" || chatID == 0 {
		return "", 0, "", errors.New("can not get chat type or chat id")
	}

	return chatType, chatID, chatName, nil
}
