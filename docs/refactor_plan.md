# IM Protocol 重构计划

## 1. 重构目标

- ✅ **单一职责原则**：将功能按职责拆分到不同模块
- ✅ **标准Go项目结构**：遵循Go项目工程最佳实践
- ✅ **简单易用接口**：对外隐藏复杂实现细节
- ✅ **保持向后兼容**：确保现有代码能平滑迁移
- ✅ **提高可维护性**：模块化设计便于后续扩展和维护

## 2. 目录结构设计

根据Go项目工程标准和单一职责原则，设计以下目录结构：

```
im-protocol/
├── cmd/                # 命令行工具（可选）
├── pkg/                # 可重用的公共包
│   ├── buffer/         # 缓冲区池管理
│   ├── codec/          # 编解码功能
│   ├── config/         # 配置常量
│   ├── core/           # 核心数据结构
│   ├── errors/         # 错误处理
│   ├── options/        # 选项模式
│   ├── stream/         # 流式处理
│   └── sync/           # 并发安全封装
├── examples/           # 示例程序
├── tests/              # 测试文件
├── go.mod              # Go模块定义
├── go.sum              # 依赖校验
└── README.md           # 项目文档
```

## 3. 模块拆分方案

### 3.1 buffer 模块

**职责**：缓冲区池管理，减少内存分配和GC压力

**文件结构**：
- `pkg/buffer/pool.go`：分级缓冲区池实现

**核心接口**：
```go
// Get 获取指定大小的缓冲区
func Get(size int) *[]byte

// Put 归还缓冲区到池中
func Put(bufPtr *[]byte)
```

**实现细节**：
- 从 `protocol.go` 中迁移 `tieredBufferPool` 实现
- 隐藏内部实现细节，对外提供简单的Get/Put接口

### 3.2 config 模块

**职责**：集中管理配置常量

**文件结构**：
- `pkg/config/config.go`：协议相关常量定义

**核心内容**：
```go
// 协议版本常量
const (
    ProtocolVersionV1      uint8 = 1
    ProtocolVersionV2      uint8 = 2
    CurrentProtocolVersion uint8 = ProtocolVersionV1
)

// 帧类型常量
const (
    FrameTypeJSON     = 1
    FrameTypeProtobuf = 2
    FrameTypeMsgPack  = 3
)

// 缓冲区大小常量
const (
    SmallBufferSize  = 2 * 1024
    MediumBufferSize = 4 * 1024
    LargeBufferSize  = 8 * 1024
)

// 其他常量...
```

### 3.3 core 模块

**职责**：核心数据结构和基础操作

**文件结构**：
- `pkg/core/frame.go`：Frame结构体定义和基础方法

**核心接口**：
```go
// New 创建新的协议帧
func New(frameType uint8, body []byte, options ...core.Option) (*Frame, error)

// Frame 协议帧接口
type Frame interface {
    GetVersion() uint8
    SetVersion(version uint8)
    GetType() uint8
    SetType(frameType uint8)
    GetBody() []byte
    SetBody(body []byte)
    GetBodyLength() uint32
    Encode() ([]byte, error)
    EncodeTo(w io.Writer) (n int, err error)
    EncodeToBytes(buf []byte) (n int, err error)
}
```

**实现细节**：
- 从 `protocol.go` 中迁移 `Frame` 结构体和相关方法
- 保持核心方法不变，优化接口设计

### 3.4 sync 模块

**职责**：并发安全封装

**文件结构**：
- `pkg/sync/frame.go`：并发安全的帧结构

**核心接口**：
```go
// New 创建新的并发安全协议帧
func New(frameType uint8, body []byte, options ...core.Option) (*SyncFrame, error)

// SyncFrame 并发安全的协议帧接口
type SyncFrame interface {
    core.Frame
    WithLock(f func(*core.Frame))
    WithRLock(f func(*core.Frame))
    Clone() *SyncFrame
}
```

**实现细节**：
- 从 `protocol.go` 中迁移 `SyncFrame` 结构体和相关方法
- 保持与core.Frame接口兼容

### 3.5 errors 模块

**职责**：统一错误处理

**文件结构**：
- `pkg/errors/errors.go`：自定义错误类型和处理函数

**核心接口**：
```go
// ProtocolError 自定义协议错误类型
type ProtocolError struct {
    Code    ErrorCode
    Message string
    Original error
}

// 错误检查函数
func IsFrameTypeError(err error) bool
func IsVersionError(err error) bool
func IsMessageTooLongError(err error) bool
func IsInvalidFrameError(err error) bool
```

**实现细节**：
- 从 `protocol.go` 中迁移错误处理相关代码
- 提供统一的错误类型和检查函数

### 3.6 options 模块

**职责**：选项模式实现

**文件结构**：
- `pkg/options/options.go`：选项接口和实现

**核心接口**：
```go
// Option 帧选项接口
type Option interface {
    applyFrame(*core.Frame) error
}

// WithVersion 设置帧版本选项
func WithVersion(version uint8) Option

// WithCopyBody 设置是否深拷贝body选项
func WithCopyBody(copy bool) Option

// WithZeroCopy 设置是否使用零拷贝模式
func WithZeroCopy(zeroCopy bool) Option
```

**实现细节**：
- 从 `protocol.go` 中迁移选项模式相关代码
- 简化接口，统一Option类型

### 3.7 codec 模块

**职责**：编解码功能

**文件结构**：
- `pkg/codec/codec.go`：编解码接口和实现

**核心接口**：
```go
// Encode 编码协议帧
func Encode(frame core.Frame) ([]byte, error)

// Decode 解码协议帧
func Decode(data []byte) (*core.Frame, error)

// EncodeTo 将帧编码并写入io.Writer
func EncodeTo(frame core.Frame, w io.Writer) (n int, err error)

// EncodeToBytes 将帧编码到提供的缓冲区
func EncodeToBytes(frame core.Frame, buf []byte) (n int, err error)
```

**实现细节**：
- 从 `protocol.go` 中迁移编解码相关代码
- 提供统一的编解码接口

### 3.8 stream 模块

**职责**：流式处理（解决TCP粘包/拆包问题）

**文件结构**：
- `pkg/stream/decoder.go`：流式解码器

**核心接口**：
```go
// NewDecoder 创建新的流解码器
func NewDecoder(options ...Option) *Decoder

// Decoder 流解码器接口
type Decoder interface {
    Feed(data []byte) error
    TryDecode() (*core.Frame, error)
    DecodeFromReader(reader io.Reader) (*core.Frame, error)
    Reset()
    Buffered() int
    Peek() []byte
    ReadFramesFromStream(reader io.Reader) ([]*core.Frame, error)
}
```

**实现细节**：
- 从 `protocol.go` 中迁移StreamDecoder相关代码
- 优化接口设计，提高易用性

## 4. 接口设计原则

### 4.1 简洁性

- 对外接口应尽可能简单，隐藏内部实现细节
- 使用函数式选项模式简化复杂配置
- 提供合理的默认值，减少配置项

### 4.2 一致性

- 保持接口命名风格一致（如使用动词+名词形式）
- 错误处理方式统一
- 参数和返回值格式一致

### 4.3 兼容性

- 确保与现有代码的向后兼容性
- 提供迁移指南和示例代码

## 5. 实现步骤

### 步骤1：创建目录结构

```bash
mkdir -p pkg/{buffer,codec,config,core,errors,options,stream,sync}
touch pkg/{buffer,codec,config,core,errors,options,stream,sync}/go.mod
```

### 步骤2：实现基础模块

1. **config模块**：迁移所有常量定义
2. **errors模块**：迁移错误处理相关代码
3. **buffer模块**：迁移缓冲区池实现

### 步骤3：实现核心模块

1. **core模块**：迁移Frame结构体和基础方法
2. **options模块**：迁移选项模式实现

### 步骤4：实现扩展模块

1. **sync模块**：迁移SyncFrame实现
2. **codec模块**：迁移编解码功能
3. **stream模块**：迁移流式处理功能

### 步骤5：实现集成接口

1. 在根目录创建 `im-protocol.go`，提供统一的对外接口
2. 实现简单易用的API，封装内部模块调用

### 步骤6：编写测试

1. 为每个模块编写单元测试
2. 编写集成测试确保模块间协作正常
3. 编写性能测试验证性能指标

## 6. 测试计划

### 6.1 单元测试

- **buffer**：测试缓冲区池的Get/Put功能
- **core**：测试Frame的创建和基本操作
- **sync**：测试并发安全操作
- **codec**：测试编解码功能和版本兼容性
- **stream**：测试流式处理和粘包/拆包处理
- **errors**：测试错误类型和检查函数

### 6.2 集成测试

- 测试完整的编解码流程
- 测试并发环境下的性能和正确性
- 测试边界情况处理

### 6.3 性能测试

- 测试缓冲区池的性能提升
- 测试编解码性能
- 测试流式处理性能

## 7. 向后兼容性

为确保现有代码能平滑迁移，提供以下兼容措施：

1. **保持API签名一致**：核心函数名和参数保持不变
2. **提供兼容层**：在根目录提供与旧版本兼容的接口
3. **迁移指南**：编写详细的迁移文档和示例代码

## 8. 预期效果

### 8.1 代码结构

- 各模块职责清晰，代码组织有序
- 遵循Go项目工程最佳实践
- 便于后续扩展和维护

### 8.2 接口易用性

```go
// 旧接口（复杂）
frame, err := protocol.NewFrame(protocol.FrameTypeJSON, []byte("hello"), protocol.WithVersion(protocol.ProtocolVersionV1))

// 新接口（简洁）
frame, err := core.New(config.FrameTypeJSON, []byte("hello"), options.WithVersion(config.ProtocolVersionV1))
```

### 8.3 性能保持

- 保持原有性能指标不变
- 可能因模块化带来轻微性能提升

## 9. 风险评估

- **接口变更**：可能需要现有代码进行少量修改
- **测试覆盖**：需要确保所有功能都有充分测试
- **性能影响**：模块化可能带来轻微的性能开销

## 10. 总结

本次重构将按照单一职责原则和Go项目工程标准，将现有代码拆分为多个功能明确的模块，同时保持接口的简单易用性。通过合理的模块拆分和接口设计，将大大提高代码的可维护性和可扩展性，为后续的功能扩展和优化奠定良好基础。