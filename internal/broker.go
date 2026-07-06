package internal

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	kafkago "github.com/segmentio/kafka-go"
)

const (
	rabbitExchange = "tabibu"
	maxRetryDelay  = 5 * time.Minute
)

// BrokerMessage is a raw message received from the broker.
type BrokerMessage struct {
	Topic   string
	Payload []byte
	Headers map[string]string
}

// BrokerHandler processes an inbound message. Non-nil return = nack/retry.
type BrokerHandler func(ctx context.Context, msg BrokerMessage) error

// StartBroker starts the broker subscription loop. It infers the provider from
// BROKER_TYPE or the URL prefix. Topics are read from the EXT_SUBSCRIBE_EVENTS
// env var (comma-separated). The consumer group is "ext.<extName>".
//
// BROKER_URL empty → returns immediately (broker disabled).
// Connection failure → retries with exponential backoff (cap: 5 min). The
// caller's context cancellation stops all retry loops.
func StartBroker(ctx context.Context, extName string, handler BrokerHandler) {
	url := os.Getenv("BROKER_URL")
	if url == "" {
		log.Println("BROKER_URL not set — broker disabled, OnEvent will not be called")
		return
	}

	topics := parseTopics(os.Getenv("EXT_SUBSCRIBE_EVENTS"))
	if len(topics) == 0 {
		log.Println("EXT_SUBSCRIBE_EVENTS not set — no broker subscriptions")
		return
	}

	bt := os.Getenv("BROKER_TYPE")
	if bt == "" {
		if strings.HasPrefix(url, "amqp") {
			bt = "rabbitmq"
		} else {
			bt = "kafka"
		}
	}

	group := "ext." + extName

	switch bt {
	case "rabbitmq":
		go runRabbitBroker(ctx, url, group, topics, handler)
	case "kafka":
		go runKafkaBroker(ctx, url, group, topics, handler)
	default:
		log.Printf("unknown BROKER_TYPE=%q — broker disabled", bt)
	}
}

// parseTopics splits a comma-separated topic list and trims whitespace.
func parseTopics(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// MarshalEvent converts a raw broker message into a JSON payload suitable for
// embedding in the Event.Payload field.
func MarshalEvent(msg BrokerMessage) ([]byte, error) {
	return json.Marshal(map[string]any{
		"topic":   msg.Topic,
		"payload": json.RawMessage(msg.Payload),
	})
}

// --- RabbitMQ ---

func runRabbitBroker(ctx context.Context, url, group string, topics []string, handler BrokerHandler) {
	delay := time.Second
	for {
		if err := subscribeRabbit(ctx, url, group, topics, handler); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			log.Printf("rabbitmq: disconnected (%v), retrying in %s", err, delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < maxRetryDelay {
				delay *= 2
			}
			continue
		}
		return // ctx cancelled
	}
}

func subscribeRabbit(ctx context.Context, url, group string, topics []string, handler BrokerHandler) error {
	conn, err := amqp.Dial(url)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(rabbitExchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("exchange declare: %w", err)
	}

	notifyClose := conn.NotifyClose(make(chan *amqp.Error, 1))

	for _, topic := range topics {
		queueName := group + "." + topic
		q, err := ch.QueueDeclare(queueName, true, false, false, false, nil)
		if err != nil {
			return fmt.Errorf("queue declare %s: %w", topic, err)
		}
		if err := ch.QueueBind(q.Name, topic, rabbitExchange, false, nil); err != nil {
			return fmt.Errorf("queue bind %s: %w", topic, err)
		}

		deliveries, err := ch.Consume(q.Name, "", false, false, false, false, nil)
		if err != nil {
			return fmt.Errorf("consume %s: %w", topic, err)
		}

		go func(topic string, deliveries <-chan amqp.Delivery) {
			for {
				select {
				case <-ctx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						return
					}
					headers := make(map[string]string, len(d.Headers))
					for k, v := range d.Headers {
						if s, ok := v.(string); ok {
							headers[k] = s
						}
					}
					msg := BrokerMessage{
						Topic:   topic,
						Payload: d.Body,
						Headers: headers,
					}
					if err := handler(ctx, msg); err != nil {
						log.Printf("rabbitmq: handler error on %s: %v — nacking", topic, err)
						d.Nack(false, true)
					} else {
						d.Ack(false)
					}
				}
			}
		}(topic, deliveries)
	}

	select {
	case <-ctx.Done():
		return nil
	case err := <-notifyClose:
		if err != nil {
			return fmt.Errorf("connection closed: %w", err)
		}
		return nil
	}
}

// --- Kafka ---

func runKafkaBroker(ctx context.Context, url, group string, topics []string, handler BrokerHandler) {
	delay := time.Second
	for {
		if err := subscribeKafka(ctx, url, group, topics, handler); err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			log.Printf("kafka: disconnected (%v), retrying in %s", err, delay)
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			if delay < maxRetryDelay {
				delay *= 2
			}
			continue
		}
		return
	}
}

func subscribeKafka(ctx context.Context, url, group string, topics []string, handler BrokerHandler) error {
	addrs := strings.Split(url, ",")

	for _, topic := range topics {
		r := kafkago.NewReader(kafkago.ReaderConfig{
			Brokers: addrs,
			Topic:   topic,
			GroupID: group,
		})

		go func(topic string, r *kafkago.Reader) {
			defer r.Close()
			for {
				m, err := r.FetchMessage(ctx)
				if err != nil {
					return
				}
				headers := make(map[string]string, len(m.Headers))
				for _, h := range m.Headers {
					headers[h.Key] = string(h.Value)
				}
				msg := BrokerMessage{
					Topic:   topic,
					Payload: m.Value,
					Headers: headers,
				}
				if err := handler(ctx, msg); err != nil {
					log.Printf("kafka: handler error on %s: %v — not committing", topic, err)
				} else {
					if err := r.CommitMessages(ctx, m); err != nil {
						log.Printf("kafka: commit error on %s: %v", topic, err)
					}
				}
			}
		}(topic, r)
	}

	<-ctx.Done()
	return nil
}
