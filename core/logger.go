package core

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Logger *zap.Logger

// InitLogger 初始化 Zap 结构化日志库 (JSON 格式输出)
func InitLogger() {
	encoderConfig := zap.NewProductionEncoderConfig()
	// 格式化时间戳
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	coreWrite := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderConfig), // 使用 JSON 编码器
		zapcore.AddSync(os.Stdout),           // 打印到标准输出
		zap.NewAtomicLevelAt(zap.InfoLevel),  // 默认级别为 Info
	)

	// 开启 Caller 追溯文件名与行数
	Logger = zap.New(coreWrite, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel))
	zap.ReplaceGlobals(Logger)
	Logger.Info("Zap 结构化日志引擎已初始化完毕.")
}
