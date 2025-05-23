package main

import (
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/paradoxe35/kproximate/config"
	"github.com/paradoxe35/kproximate/logger"
	"github.com/paradoxe35/kproximate/rabbitmq"
	"github.com/paradoxe35/kproximate/scaler"
	amqp "github.com/rabbitmq/amqp091-go"
)

// Global retry tracking for node-specific delays
var (
	nodeRetryTracker = make(map[string]int)
	nodeRetryMutex   sync.RWMutex
)

// incrementNodeRetryCount increments the retry count for a specific node
func incrementNodeRetryCount(nodeName string) int {
	nodeRetryMutex.Lock()
	defer nodeRetryMutex.Unlock()
	nodeRetryTracker[nodeName]++
	return nodeRetryTracker[nodeName]
}

// resetNodeRetryCount resets the retry count for a specific node (on success)
func resetNodeRetryCount(nodeName string) {
	nodeRetryMutex.Lock()
	defer nodeRetryMutex.Unlock()
	delete(nodeRetryTracker, nodeName)
}

func main() {
	kpConfig, err := config.GetKpConfig()
	if err != nil {
		logger.ErrorLog("Failed to get config", "error", err)
	}

	logger.ConfigureLogger("worker", kpConfig.Debug)

	scaler, err := scaler.NewProxmoxScaler(kpConfig)
	if err != nil {
		logger.ErrorLog("Failed to initialise scaler", "error", err)
	}

	rabbitConfig, err := config.GetRabbitConfig()
	if err != nil {
		logger.ErrorLog("Failed to get rabbit config", "error", err)
	}

	conn, _ := rabbitmq.NewRabbitmqConnection(rabbitConfig)
	defer conn.Close()

	scaleUpChannel := rabbitmq.NewChannel(conn)
	defer scaleUpChannel.Close()
	scaleUpQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleUpEvents")
	err = scaleUpChannel.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		logger.ErrorLog("Failed to set scale up QoS", "error", err)
	}

	scaleDownChannel := rabbitmq.NewChannel(conn)
	defer scaleDownChannel.Close()
	scaleDownQueue := rabbitmq.DeclareQueue(scaleUpChannel, "scaleDownEvents")
	err = scaleDownChannel.Qos(
		1,
		0,
		false,
	)
	if err != nil {
		logger.ErrorLog("Failed to set scale down QoS", "error", err)
	}

	scaleUpMsgs, err := scaleUpChannel.Consume(
		scaleUpQueue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.ErrorLog("Failed to register scale up consumer", "error", err)
	}

	scaleDownMsgs, err := scaleDownChannel.Consume(
		scaleDownQueue.Name,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		logger.ErrorLog("Failed to register scale down consumer", "error", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	sigChan := make(chan os.Signal, 1)
	go func() {
		<-sigChan
		cancel()
	}()

	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	logger.InfoLog("Listening for scale events")

	for {
		select {
		case scaleUpMsg := <-scaleUpMsgs:
			consumeScaleUpMsg(ctx, scaler, scaleUpMsg)

		case scaleDownMsg := <-scaleDownMsgs:
			consumeScaleDownMsg(ctx, scaler, scaleDownMsg)

		case <-ctx.Done():
			return
		}
	}
}

func consumeScaleUpMsg(ctx context.Context, kpScaler scaler.Scaler, scaleUpMsg amqp.Delivery) {
	var scaleUpEvent *scaler.ScaleEvent
	err := json.Unmarshal(scaleUpMsg.Body, &scaleUpEvent)
	if err != nil {
		logger.ErrorLog("Failed to unmarshal scale up event", "error", err.Error())
		scaleUpMsg.Reject(false) // Don't requeue malformed messages
		return
	}

	nodeName := scaleUpEvent.NodeName
	retryNodeName := "scale-up-" + nodeName

	if scaleUpMsg.Redelivered {
		// Increment retry count for this specific node
		retryCount := incrementNodeRetryCount(retryNodeName)
		logger.InfoLog("Retrying scale up event", "nodeName", nodeName, "retryCount", retryCount)

		// CRITICAL: ScaleUp always creates a new node, so we MUST clean up any existing
		// node with the same name before retrying, otherwise we'll get name conflicts
		logger.InfoLog("Cleaning up existing node before retry", "nodeName", nodeName)
		kpScaler.DeleteNode(ctx, nodeName)

		// Apply node-specific exponential backoff delay
		delay := calculateRetryDelay(retryCount)
		logger.InfoLog("Applying node-specific retry delay", "nodeName", nodeName, "delay", delay, "retryCount", retryCount)
		time.Sleep(delay)
	} else {
		logger.InfoLog("Triggered scale up event", "nodeName", nodeName)
	}

	// Attempt the scale up operation
	err = kpScaler.ScaleUp(ctx, scaleUpEvent)
	if err != nil {
		logger.WarnLog("Scale up event failed", "error", err.Error(), "nodeName", nodeName)

		// Always clean up the failed node attempt since ScaleUp may have partially created resources
		kpScaler.DeleteNode(ctx, nodeName)

		// Requeue for retry (RabbitMQ's delivery limit will prevent infinite loops)
		scaleUpMsg.Reject(true)
		return
	}

	// Success case - reset retry delay for this specific node
	resetNodeRetryCount(retryNodeName)
	logger.InfoLog("Successfully scaled up node", "nodeName", nodeName)
	scaleUpMsg.Ack(false)
}

func consumeScaleDownMsg(ctx context.Context, kpScaler scaler.Scaler, scaleDownMsg amqp.Delivery) {
	var scaleDownEvent *scaler.ScaleEvent
	err := json.Unmarshal(scaleDownMsg.Body, &scaleDownEvent)
	if err != nil {
		logger.ErrorLog("Failed to unmarshal scale down event", "error", err.Error())
		scaleDownMsg.Reject(false)
		return
	}

	nodeName := scaleDownEvent.NodeName
	retryNodeName := "scale-down-" + nodeName

	if scaleDownMsg.Redelivered {
		// Increment retry count for this specific node
		retryCount := incrementNodeRetryCount(retryNodeName)
		logger.InfoLog("Retrying scale down event", "nodeName", nodeName, "retryCount", retryCount)

		// Apply node-specific exponential backoff delay
		delay := calculateRetryDelay(retryCount)
		logger.InfoLog("Applying node-specific retry delay", "nodeName", nodeName, "delay", delay, "retryCount", retryCount)
		time.Sleep(delay)
	} else {
		logger.InfoLog("Triggered scale down event", "nodeName", nodeName)
	}

	scaleCtx, scaleCancel := context.WithDeadline(ctx, time.Now().Add(time.Second*300))
	defer scaleCancel()

	err = kpScaler.ScaleDown(scaleCtx, scaleDownEvent)
	if err != nil {
		logger.WarnLog("Scale down event failed", "error", err.Error(), "nodeName", nodeName)

		// Requeue for retry (RabbitMQ's delivery limit will prevent infinite loops)
		scaleDownMsg.Reject(true)
		return
	}

	resetNodeRetryCount(retryNodeName)
	logger.InfoLog("Successfully scaled down node", "nodeName", nodeName)
	scaleDownMsg.Ack(false)
}

// Constants for retry configuration
const (
	baseRetryDelaySec = 1    // 1 second
	maxRetryDelaySec  = 1800 // 30 minutes
)

// calculateRetryDelay implements exponential backoff
func calculateRetryDelay(retryCount int) time.Duration {
	if retryCount <= 0 {
		return 0
	}

	// Exponential backoff: baseDelay * 2^(retryCount-1)
	delaySec := baseRetryDelaySec * (1 << (retryCount - 1))

	// Cap the delay
	if delaySec > maxRetryDelaySec {
		delaySec = maxRetryDelaySec
	}

	return time.Duration(delaySec) * time.Second
}
