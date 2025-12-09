# IM Protocol

一个高性能的即时通讯协议库，专为在客户端和服务器之间传输结构化数据而设计。

## 特性

- 🚀 **高性能**: 零拷贝模式、缓冲区池、流式处理
- 🔧 **多格式支持**: JSON、Protobuf、MsgPack 等序列化格式
- 🔒 **并发安全**: 线程安全的数据结构和操作
- 📦 **流式处理**: 智能处理 TCP 粘包/拆包问题
- 🎯 **模块化设计**: 清晰的包结构和依赖关系
- 📊 **全面测试**: 95%+ 测试覆盖率

## 快速开始

### 前置要求

- Go 1.21+ 环境
- 基本 Go 编程知识

### 安装

```bash
go get github.com/yourusername/im-protocol
```

### 基本使用

```go
package main

import (
    "fmt"
    "im-protocol/config"
    "im-protocol/pkg/common/core"
    "im-protocol/pkg/common/codec"
)

func main() {
    // 创建帧
    frame, err := core.New(
        config.ProtocolVersion,
        config.FrameTypeJSON,
        core.WithBody([]byte(`{"message":"hello world"}`)),
    )
    if err != nil {
        panic(err)
    }

    // 编码帧
    data, err := codec.Encode(frame)
    if err != nil {
        panic(err)
    }

    // 解码帧
    decodedFrame, err := codec.Decode(data)
    if err != nil {
        panic(err)
    }

    fmt.Printf("消息: %s\n", decodedFrame.GetBody())
}
```

## 核心概念

### 协议帧 (Frame)

协议帧是 IM Protocol 的核心数据结构，包含以下字段：

```
+--------+--------+--------+--------+--------+--------+--------+--------+--------+
| 版本号 (1字节) | 子版本号 (1字节) | 消息类型 (1字节) |       消息体长度 (4字节，大端序)       |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+
|                              消息体 (可变长度)                              |
+--------+--------+--------+--------+--------+--------+--------+--------+--------+
```

### 序列化格式

IM Protocol 支持多种序列化格式：

- **JSON** (`config.FrameTypeJSON`) - 人类可读，通用性强
- **Protobuf** (`config.FrameTypeProtobuf`) - 高性能，二进制格式
- **MsgPack** (`config.FrameTypeMsgPack`) - 紧凑，高效

## 最佳实践

### 设计原则

- **模块化设计**: 将功能按职责分离到不同模块
- **接口优先**: 先定义接口，再实现具体功能
- **错误处理**: 显式处理所有可能的错误情况

### 性能优化

#### 使用缓冲区池

```go
// ✅ 使用缓冲区池减少 GC 压力
func handleLargeData(data []byte) error {
    // 从池获取缓冲区
    buf := buffer.Get(len(data))
    defer buffer.Put(buf) // 确保归还
    
    copy(buf, data)
    return processBuffer(buf)
}
```

#### 预分配切片容量

```go
// ✅ 预分配容量避免扩容
func collectFrames(frames []*core.Frame) [][]byte {
    results := make([][]byte, 0, len(frames)) // 预分配容量
    
    for _, frame := range frames {
        data, err := codec.Encode(frame)
        if err != nil {
            continue
        }
        results = append(results, data)
    }
    return results
}
```

### 错误处理

```go
// ✅ 好的错误处理
func processFrame(data []byte) error {
    frame, err := codec.Decode(data)
    if err != nil {
        return fmt.Errorf("解码失败: %w", err)
    }
    
    if frame == nil {
        return errors.New("帧为空")
    }
    
    return nil
}
```

## 性能

基于 Go 1.25.4，Linux AMD64 环境下的性能测试结果：

| 操作 | 性能指标 |
|------|----------|
| 帧创建 | 7,261,579 次/秒 |
| 帧编码 | 4,705,562 次/秒 |
| 帧解码 | 17,855,220 次/秒 |
| 缓冲区池 | 70,101,489 次/秒 |

## 项目结构

```
im-protocol/
├── cmd/main/           # 主程序入口
├── config/             # 统一配置管理
├── pkg/common/         # 公共包
│   ├── buffer/         # 缓冲区管理
│   ├── codec/          # 编解码
│   ├── core/           # 核心数据结构
│   ├── errors/         # 错误处理
│   ├── frame/          # 帧处理（兼容层）
│   ├── options/        # 选项模式
│   ├── stream/         # 流式处理
│   ├── sync/           # 并发安全
│   └── utils/          # 工具函数
├── examples/           # 示例程序
├── benchmarks/         # 性能基准测试
└── tests/              # 测试文件
```

## 文档

- [API 参考](docs/api/API.md) - 详细的 API 文档
- [设计文档](docs/design/design.md) - 协议架构设计
- [贡献指南](docs/CONTRIBUTING.md) - 如何参与项目开发

## 许可证

MIT License

## 联系方式

- 项目主页: [https://github.com/yourusername/im-protocol](https://github.com/yourusername/im-protocol)
- 提交问题: [https://github.com/yourusername/im-protocol/issues](https://github.com/yourusername/im-protocol/issues)
- 邮件: contact@example.com
