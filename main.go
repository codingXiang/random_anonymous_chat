package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/codingXiang/configer"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	"gopkg.in/olahol/melody.v1"
	"log"
	"net/http"
)

type Message struct {
	Event   string `json:"event"`
	Name    string `json:"name"`
	Content string `json:"content"`
}
var (
	KEY  = ""
	WAIT = ""
)


func NewMessage(event, name, content string) *Message {
	return &Message{
		Event:   event,
		Name:    name,
		Content: content,
	}
}

func (m *Message) GetByteMessage() []byte {
	result, _ := json.Marshal(m)
	return result
}

var redisClient *redis.Client
var conf *viper.Viper

func init() {
	config := configer.NewConfigerCore("yaml", "config", "./config", ".")
	config.SetAutomaticEnv("")

	if c, err := config.ReadConfig(nil); err == nil {
		conf = c
		KEY = conf.GetString("application.chat.id")
		WAIT = conf.GetString("application.chat.wait")
	} else {
		panic(err)
	}

	redisClient = redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%d", conf.GetString("redis.url"), conf.GetInt("redis.port")),
		Password: conf.GetString("redis.password"), // no password set
		DB:       conf.GetInt("redis.db"),        // use default DB
	})

	pong, err := redisClient.Ping(context.Background()).Result()
	if err == nil {
		log.Println("redis 回應成功，", pong)
	} else {
		log.Fatal("redis 無法連線，錯誤為", err)
	}
}

func main() {


	r := gin.Default()
	r.LoadHTMLGlob("template/html/*")
	r.Static("/assets", "./template/assets")
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	m := melody.New()
	r.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		id := GetSessionID(s)
		chatTo, _ := redisClient.Get(context.TODO(), id).Result()
		m.BroadcastFilter(msg, func(session *melody.Session) bool {
			compareID, _ := session.Get(KEY)
			return compareID == chatTo || compareID == id
		})
	})

	m.HandleConnect(func(session *melody.Session) {
		id := InitSession(session)
		if key, err := GetWaitFirstKey(); err == nil && key != "" {
			CreateChat(id, key)
			msg := NewMessage("other", "對方已經", "加入聊天室").GetByteMessage()
			m.BroadcastFilter(msg, func(session *melody.Session) bool {
				compareID, _ := session.Get(KEY)
				return compareID == id || compareID == key
			})
		} else {
			AddToWaitList(id)
		}
	})

	m.HandleClose(func(session *melody.Session, i int, s string) error {
		id := GetSessionID(session)
		chatTo, _ := redisClient.Get(context.TODO(), id).Result()
		msg := NewMessage("other", "對方已經", "離開聊天室").GetByteMessage()
		RemoveChat(id, chatTo)
		return m.BroadcastFilter(msg, func(session *melody.Session) bool {
			compareID, _ := session.Get(KEY)
			return compareID == chatTo
		})
	})
	r.Run(fmt.Sprintf(":%d", conf.GetInt("application.port")))
}

func AddToWaitList(id string) error {
	return redisClient.LPush(context.Background(), WAIT, id).Err()
}

func GetWaitFirstKey() (string, error) {
	return redisClient.LPop(context.Background(), WAIT).Result()
}

func CreateChat(id1, id2 string) {
	redisClient.Set(context.Background(), id1, id2, 0)
	redisClient.Set(context.Background(), id2, id1, 0)
}

func RemoveChat(id1, id2 string) {
	redisClient.Del(context.Background(), id1, id2)
}
func GetSessionID(s *melody.Session) string {
	if id, isExist := s.Get(KEY); isExist {
		return id.(string)
	}
	return InitSession(s)
}

func InitSession(s *melody.Session) string {
	id := uuid.New().String()
	s.Set(KEY, id)
	return id
}
