package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/storage/memory/v2"
	"github.com/gorilla/websocket"
)

var addr = flag.String("addr", "bundles-api-rest.jito.wtf", "http service address")

type Bundle struct {
	Time                     string  `json:"time"`
	LandedTips25thPercentile float64 `json:"landed_tips_25th_percentile"`
	LandedTips50thPercentile float64 `json:"landed_tips_50th_percentile"`
	LandedTips75thPercentile float64 `json:"landed_tips_75th_percentile"`
	LandedTips95thPercentile float64 `json:"landed_tips_95th_percentile"`
	LandedTips99thPercentile float64 `json:"landed_tips_99th_percentile"`
}

type Information struct {
	Repository   string `json:"repository"`
	Author       string `json:"author"`
	Language     string `json:"language"`
	SubscribedTo string `json:"subscribed_to"`
}

type BundleResponse struct {
	Bundle
	Annotations Information `json:"annotations"`
}

func main() {
	app := fiber.New()

	store := memory.New(memory.Config{
		GCInterval: time.Minute * 5,
	})

	app.Get("/", func(c *fiber.Ctx) error {
		cached, err := store.Get("current")
		if err != nil {
			log.Println("get:", err)
			return c.Status(500).JSON(map[string]string{"error": err.Error(), "description": "Could not get the cached data. Probably the program did not have enough time to write the data to the cache."})
		}

		var data Bundle

		if err = json.Unmarshal(cached, &data); err != nil {
			log.Println("parse:", err)
			return c.Status(500).JSON(map[string]string{"error": err.Error(), "description": "Could not parse the cached data. Probably the program did not have enough time to write the data to the cache."})
		}

		return c.JSON(BundleResponse{
			data,
			Information{
				"https://github.com/kyee-rs/fuji",
				"Dutch (@devkyee on TG)",
				"Go",
				"ws://bundles-api-rest.jito.wtf/api/v1/bundles/tip_stream",
			},
		})
	})

	go func() {
		if err := app.Listen(":8080"); err != nil {
			log.Fatal(err)
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: *addr, Path: "/api/v1/bundles/tip_stream"}
	log.Printf("connecting to %s", u.String())

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)

	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}

			str_message := string(message[:])
			// Parse json
			var data []Bundle

			err = json.Unmarshal([]byte(str_message), &data)

			if err != nil {
				log.Println("parse:", err)
				return
			}
			log.Printf("Received: %+v", data)
			bytes, err := json.Marshal(data[0])

			if err != nil {
				log.Println("marshal:", err)
				return
			}

			store.Set("current", bytes, time.Minute*5)
		}
	}()

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")

			// Cleanly close the connection by sending a close message and then
			// waiting (with timeout) for the server to close the connection.
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}
