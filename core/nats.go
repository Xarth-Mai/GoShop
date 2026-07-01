package core

import (
	"errors"
	"time"

	"GoShop/config"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

var (
	NATSConn  *nats.Conn
	JetStream nats.JetStreamContext
)

// InitNATS 初始化 NATS 及其 JetStream 持久化流
func InitNATS() error {
	url := nats.DefaultURL
	if config.GlobalConfig != nil && config.GlobalConfig.NATS.URL != "" {
		url = config.GlobalConfig.NATS.URL
	}

	var err error
	// 开启自愈自动重连能力
	NATSConn, err = nats.Connect(url,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(nc *nats.Conn, err error) {
			Logger.Warn("NATS 链接断开，准备自动重连", zap.Error(err))
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			Logger.Info("NATS 重新连接成功", zap.String("url", nc.ConnectedUrl()))
		}),
	)
	if err != nil {
		return err
	}

	JetStream, err = NATSConn.JetStream()
	if err != nil {
		return err
	}

	Logger.Info("NATS JetStream 初始化完成", zap.String("url", url))
	return nil
}

// CreateOrUpdateStream 声明或更新持久化 Stream 资源
func CreateOrUpdateStream(streamName string, subjects []string) error {
	if JetStream == nil {
		return errors.New("nats: no connection")
	}

	streamConfig := &nats.StreamConfig{
		Name:     streamName,
		Subjects: subjects,
		Storage:  nats.FileStorage, // 本地演示和生产均用 File 保证持久化安全
		Replicas: 1,                // 演示默认单副本，生产集群可调成 3
	}

	_, err := JetStream.AddStream(streamConfig)
	if err == nil {
		return nil
	}

	// 如果 Stream 已经存在但配置不一致，则进行更新
	_, err = JetStream.UpdateStream(streamConfig)
	if err != nil {
		Logger.Warn("更新 NATS Stream 失败，尝试删除重修", zap.String("stream", streamName), zap.Error(err))
		// 容错自愈：删除后重建
		_ = JetStream.DeleteStream(streamName)
		_, err = JetStream.AddStream(streamConfig)
	}

	return err
}

// RegisterSubscriber 订阅消息主题，并自动分发消费
func RegisterSubscriber(subject, queueGroup, consumerName string, handler func(msgData []byte) error) (*nats.Subscription, error) {
	if JetStream == nil {
		return nil, errors.New("nats: no connection")
	}

	return JetStream.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		err := handler(msg.Data)
		if err != nil {
			Logger.Warn("NATS 消费处理失败，消息将被重新投递", zap.String("subject", subject), zap.Error(err))
			msg.Nak() // 通知 NATS 消息处理失败，触发重新投递
			return
		}
		msg.Ack() // 手动 Ack 确认
	}, nats.ManualAck(), nats.DeliverNew())
}
