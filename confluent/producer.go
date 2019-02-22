package confluent

import (
	"fmt"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

//KafkaProducer holds the kafka producer and some message counters
type KafkaProducer struct {
	SuccessCount int64
	FailedCount  int64
	MessageCount int64
	Topic        string
	ClientID     string
	Producer     *kafka.Producer
	RateLimiter  RateLimiter
}

//NewKafkaProducer creates a new producer
func NewKafkaProducer(topic string, clientID string) (*KafkaProducer, error) {
	kp := &KafkaProducer{
		Topic:    topic,
		ClientID: clientID,
	}

	var err error

	kp.Producer, err = kafka.NewProducer(
		&kafka.ConfigMap{
			"bootstrap.servers":                     "localhost",
			"acks":                                  "all",
			"compression.type":                      "lz4",
			"retries":                               10000000,
			"client.id":                             clientID,
			"max.in.flight.requests.per.connection": 5,
			"enable.idempotence":                    true,
		})
	if err != nil {
		return nil, fmt.Errorf("cannot create new producer error [%#v]", err)
	}

	// Delivery report handler for produced messages
	go func() {
		for e := range kp.Producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					kp.FailedCount++
				} else {
					kp.SuccessCount++
				}
				kp.MessageCount--
			}
		}
	}()

	return kp, nil
}

//RateLimiter limites the producer to send only messages per second or wait
type RateLimiter interface {
	Check(now time.Time) time.Duration
}

//SetRateLimiter sets the rate limiter to use
func (kp *KafkaProducer) SetRateLimiter(rateLimiter RateLimiter) {
	kp.RateLimiter = rateLimiter
}

//Close the producer
func (kp *KafkaProducer) Close() {
	kp.Producer.Close()
}

//SendKeyValue send message with key and value
func (kp *KafkaProducer) SendKeyValue(key []byte, value []byte) error {
	if kp.RateLimiter != nil {
		idleTime := kp.RateLimiter.Check(time.Now())
		if idleTime > 0 {
			time.Sleep(idleTime)
		}
	}
	err := kp.Producer.Produce(
		&kafka.Message{
			TopicPartition: kafka.TopicPartition{
				Topic:     &kp.Topic,
				Partition: kafka.PartitionAny,
			},
			Key:   key,
			Value: value,
		},
		nil)
	if err != nil {
		kp.MessageCount++
	}

	return err

}

//WaitUntilSendComplete wait until all messages are sent
func (kp *KafkaProducer) WaitUntilSendComplete() {
	for kp.MessageCount > 0 {
		time.Sleep(time.Second * 1)
	}
}
