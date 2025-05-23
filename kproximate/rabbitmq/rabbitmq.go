package rabbitmq

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/logger"
	amqp "github.com/rabbitmq/amqp091-go"
)

type queueInfo struct {
	MessagesUnacknowledged int `json:"messages_unacknowledged,omitempty"`
}

func NewRabbitmqConnection(rabbitConfig config.RabbitConfig) (*amqp.Connection, *http.Client) {
	tlsConfig := &tls.Config{InsecureSkipVerify: true}
	var conn *amqp.Connection
	var err error
	var mgmtClient *http.Client

	if rabbitConfig.UseTLS {
		// Use TLS connection
		rabbitMQUrl := fmt.Sprintf("amqps://%s:%s@%s:%d/", rabbitConfig.User, rabbitConfig.Password, rabbitConfig.Host, rabbitConfig.Port)
		logger.InfoLog("Connecting to RabbitMQ with TLS", "url", rabbitMQUrl)

		conn, err = amqp.DialTLS(rabbitMQUrl, tlsConfig)

		// Create HTTP client with TLS for management API
		tr := &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		mgmtClient = &http.Client{
			Transport: tr,
		}
	} else {
		// Use non-TLS connection
		rabbitMQUrl := fmt.Sprintf("amqp://%s:%s@%s:%d/", rabbitConfig.User, rabbitConfig.Password, rabbitConfig.Host, rabbitConfig.Port)
		logger.InfoLog("Connecting to RabbitMQ without TLS", "url", rabbitMQUrl)

		conn, err = amqp.Dial(rabbitMQUrl)

		// Create standard HTTP client for management API
		mgmtClient = &http.Client{}
	}

	if err != nil {
		logger.ErrorLog("Failed to connect to RabbitMQ", "error", err)
	}

	return conn, mgmtClient
}

func NewChannel(conn *amqp.Connection) *amqp.Channel {
	ch, err := conn.Channel()
	if err != nil {
		logger.ErrorLog("Failed to open a channel", "error", err)
	}

	return ch
}

func DeclareQueue(ch *amqp.Channel, queueName string) *amqp.Queue {
	args := amqp.Table{
		"x-queue-type":     "quorum",
		"x-delivery-limit": 2,
	}

	q, err := ch.QueueDeclare(
		queueName, // name
		true,      // durable
		false,     // delete when unused
		false,     // exclusive
		false,     // no-wait
		args,      // arguments
	)
	if err != nil {
		logger.ErrorLog("Failed to declare a queue", "error", err)
	}

	return &q
}

func GetPendingScaleEvents(ch *amqp.Channel, queueName string) (int, error) {
	args := amqp.Table{
		"x-queue-type":     "quorum",
		"x-delivery-limit": 2,
	}
	scaleEvents, err := ch.QueueDeclarePassive(
		queueName,
		true,  // durable
		false, // delete when unused
		false, // exclusive
		false, // no-wait
		args,  // arguments
	)
	if err != nil {
		return 0, err
	}

	return scaleEvents.Messages, nil
}

func GetRunningScaleEvents(client *http.Client, rabbitConfig config.RabbitConfig, queueName string) (int, error) {
	endpoint := fmt.Sprintf("http://%s:15672/api/queues/%s/%s", rabbitConfig.Host, url.PathEscape("/"), queueName)
	logger.InfoLog("Connecting to RabbitMQ management API", "url", endpoint)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return 0, err
	}

	req.Close = true
	req.SetBasicAuth(rabbitConfig.User, rabbitConfig.Password)

	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	var queueInfo queueInfo

	err = json.NewDecoder(res.Body).Decode(&queueInfo)
	if err != nil {
		return 0, err
	}

	return queueInfo.MessagesUnacknowledged, nil
}
