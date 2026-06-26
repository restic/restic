package fs

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/restic/restic/internal/data"
)

// S3Config 存储 S3 兼容对象存储的连接配置
type S3Config struct {
	Endpoint  string // S3 服务地址，如 "minio.local:9000"
	UseHTTP   bool   // 是否使用 HTTP（非 HTTPS）
	Bucket    string // 桶名称
	Prefix    string // 对象前缀，用于限定备份范围
	Region    string // S3 区域
	AccessKey string // 访问密钥 ID
	SecretKey string // 访问密钥 Secret
}

// s3FS 实现了 FS 接口，用于从 S3 兼容对象存储读取文件
type s3FS struct {
	client *minio.Client // minio S3 客户端
	bucket string        // 桶名称
	prefix string        // 对象前缀，非空时以 "/" 结尾
}

// 编译时检查 s3FS 是否实现了 FS 接口
var _ FS = &s3FS{}

// NewS3FS 创建一个基于 S3 兼容对象存储的文件系统实例
// 支持静态密钥、AWS 环境变量、MinIO 环境变量三种凭证方式
func NewS3FS(cfg S3Config) (FS, error) {
	// 构建凭证链：静态密钥 > AWS 环境变量 > MinIO 环境变量
	creds := credentials.NewChainCredentials([]credentials.Provider{
		&credentials.Static{
			Value: credentials.Value{
				AccessKeyID:     cfg.AccessKey,
				SecretAccessKey: cfg.SecretKey,
			},
		},
		&credentials.EnvAWS{},
		&credentials.EnvMinio{},
	})

	opts := &minio.Options{
		Creds:  creds,
		Secure: !cfg.UseHTTP,
		Region: cfg.Region,
	}

	client, err := minio.New(cfg.Endpoint, opts)
	if err != nil {
		return nil, fmt.Errorf("s3fs: minio.New: %w", err)
	}

	// 确保前缀以 "/" 结尾
	prefix := cfg.Prefix
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &s3FS{
		client: client,
		bucket: cfg.Bucket,
		prefix: prefix,
	}, nil
}

// cleanPath 将路径规范化为以 "/" 开头的绝对路径
func (fs *s3FS) cleanPath(name string) string {
	name = path.Clean("/" + name)
	if name == "/" {
		return "/"
	}
	return name
}

// objectKey 将规范化后的路径转换为 S3 对象键
// 例如：prefix="data/", name="/file.txt" → "data/file.txt"
func (fs *s3FS) objectKey(name string) string {
	if name == "/" {
		// 根路径：返回前缀（去掉末尾 "/"），空前缀时返回空字符串
		return strings.TrimSuffix(fs.prefix, "/")
	}
	key := strings.TrimPrefix(name, "/")
	return fs.prefix + key
}

// OpenFile 打开 S3 上的文件或目录
// metadataOnly 为 true 时仅获取元数据，不下载文件内容（性能优化）
// flag 仅支持 O_RDONLY、O_NOFOLLOW、O_DIRECTORY
func (fs *s3FS) OpenFile(name string, flag int, metadataOnly bool) (File, error) {
	if flag&^(O_RDONLY|O_NOFOLLOW|O_DIRECTORY) != 0 {
		return nil, pathError("open", name, fmt.Errorf("invalid combination of flags 0x%x", flag))
	}

	name = fs.cleanPath(name)
	key := fs.objectKey(name)

	ctx := context.Background()

	// 根路径始终视为目录
	if name == "/" {
		return fs.openDir(ctx, name, key)
	}

	// 先尝试作为对象查找（StatObject 不下载内容）
	objInfo, err := fs.client.StatObject(ctx, fs.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		// 找到对象，返回文件
		fi := fs.objectToFileInfo(name, objInfo)
		return &s3File{
			fs:           fs,
			name:         name,
			key:          key,
			fi:           fi,
			metadataOnly: metadataOnly,
		}, nil
	}

	// 未找到对象，尝试作为目录（检查是否有以此为前缀的子对象）
	isDir, children, dirModTime := fs.listDir(ctx, key)
	if isDir {
		fi := &ExtendedFileInfo{
			Name:    path.Base(name),
			Mode:    os.ModeDir | 0755,
			ModTime: dirModTime,
			Size:    0,
			Links:   1,
			UID:     uint32(os.Getuid()),
			GID:     uint32(os.Getgid()),
		}
		return &s3Dir{
			name:     name,
			fi:       fi,
			entries:  children,
			metadata: true,
		}, nil
	}

	return nil, pathError("open", name, syscall.ENOENT)
}

// openDir 打开一个目录，列出其子条目
func (fs *s3FS) openDir(ctx context.Context, name string, key string) (File, error) {
	isDir, children, dirModTime := fs.listDir(ctx, key)
	if !isDir {
		// 根路径在桶为空时也应存在
		if name == "/" {
			return &s3Dir{
				name: name,
				fi: &ExtendedFileInfo{
					Name:    "",
					Mode:    os.ModeDir | 0755,
					ModTime: time.Now(),
					Size:    0,
					Links:   1,
					UID:     uint32(os.Getuid()),
					GID:     uint32(os.Getgid()),
				},
				entries:  children,
				metadata: true,
			}, nil
		}
		return nil, pathError("open", name, syscall.ENOENT)
	}

	return &s3Dir{
		name: name,
		fi: &ExtendedFileInfo{
			Name:    path.Base(name),
			Mode:    os.ModeDir | 0755,
			ModTime: dirModTime,
			Size:    0,
			Links:   1,
			UID:     uint32(os.Getuid()),
			GID:     uint32(os.Getgid()),
		},
		entries:  children,
		metadata: true,
	}, nil
}

// Lstat 获取 S3 上文件或目录的元数据信息，不跟踪符号链接
func (fs *s3FS) Lstat(name string) (*ExtendedFileInfo, error) {
	name = fs.cleanPath(name)

	// 根路径始终返回目录信息
	if name == "/" {
		return &ExtendedFileInfo{
			Name:    "",
			Mode:    os.ModeDir | 0755,
			ModTime: time.Now(),
			Size:    0,
			Links:   1,
			UID:     uint32(os.Getuid()),
			GID:     uint32(os.Getgid()),
		}, nil
	}

	key := fs.objectKey(name)
	ctx := context.Background()

	// 先尝试作为对象获取元数据
	objInfo, err := fs.client.StatObject(ctx, fs.bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return fs.objectToFileInfo(name, objInfo), nil
	}

	// 再尝试作为目录
	isDir, _, dirModTime := fs.listDir(ctx, key)
	if isDir {
		return &ExtendedFileInfo{
			Name:    path.Base(name),
			Mode:    os.ModeDir | 0755,
			ModTime: dirModTime,
			Size:    0,
			Links:   1,
			UID:     uint32(os.Getuid()),
			GID:     uint32(os.Getgid()),
		}, nil
	}

	return nil, pathError("lstat", name, os.ErrNotExist)
}

// Join 将多个路径元素连接为一个路径
func (fs *s3FS) Join(elem ...string) string {
	return path.Join(elem...)
}

// Separator 返回路径分隔符，S3 使用 "/"
func (fs *s3FS) Separator() string {
	return "/"
}

// Abs 返回绝对路径，S3 路径始终为绝对路径
func (fs *s3FS) Abs(p string) (string, error) {
	return fs.cleanPath(p), nil
}

// Clean 清理路径，去除多余的 "." 和 ".."
func (fs *s3FS) Clean(p string) string {
	return path.Clean(p)
}

// VolumeName 返回卷名，S3 无此概念，返回空字符串
func (fs *s3FS) VolumeName(_ string) string {
	return ""
}

// IsAbs 判断是否为绝对路径，S3 路径始终为绝对路径
func (fs *s3FS) IsAbs(_ string) bool {
	return true
}

// Dir 返回路径的目录部分
func (fs *s3FS) Dir(p string) string {
	return path.Dir(p)
}

// Base 返回路径的最后一个元素
func (fs *s3FS) Base(p string) string {
	return path.Base(p)
}

// objectToFileInfo 将 S3 ObjectInfo 转换为 ExtendedFileInfo
// S3 没有 Unix 权限和 inode 概念，使用合理默认值填充
func (fs *s3FS) objectToFileInfo(name string, info minio.ObjectInfo) *ExtendedFileInfo {
	return &ExtendedFileInfo{
		Name:       path.Base(name),
		Mode:       0644, // S3 对象默认权限
		Size:       info.Size,
		ModTime:    info.LastModified,
		AccessTime: info.LastModified,
		ChangeTime: info.LastModified,
		Links:      1,
		UID:        uint32(os.Getuid()), // 使用当前进程的 uid
		GID:        uint32(os.Getgid()), // 使用当前进程的 gid
	}
}

// listDir 列出指定前缀下的子对象和子目录
// S3 是扁平命名空间，通过 delimiter="/" 模拟目录结构
// 返回值：(是否为目录, 子条目名称列表, 最新修改时间)
func (fs *s3FS) listDir(ctx context.Context, key string) (bool, []string, time.Time) {
	prefix := key
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	var children []string
	childSet := make(map[string]bool) // 用于去重
	var latestMod time.Time

	// 使用 delimiter 获取直接子对象和公共前缀（模拟子目录）
	opts := minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false, // 非递归，只获取当前层级
	}

	for obj := range fs.client.ListObjects(ctx, fs.bucket, opts) {
		if obj.Err != nil {
			return false, nil, time.Time{}
		}

		// 公共前缀（以 "/" 结尾）视为子目录
		if obj.Key != "" && strings.HasSuffix(obj.Key, "/") {
			dirName := strings.TrimSuffix(strings.TrimPrefix(obj.Key, prefix), "/")
			if dirName != "" && !childSet[dirName] {
				// 对于嵌套前缀（如 "sub/deep/"），只取第一级 "sub"
				if idx := strings.Index(dirName, "/"); idx >= 0 {
					dirName = dirName[:idx]
				}
				if !childSet[dirName] {
					childSet[dirName] = true
					children = append(children, dirName)
				}
			}
			continue
		}

		// 普通对象
		name := strings.TrimPrefix(obj.Key, prefix)
		if name == "" {
			continue
		}
		if !childSet[name] {
			childSet[name] = true
			children = append(children, name)
		}

		if obj.LastModified.After(latestMod) {
			latestMod = obj.LastModified
		}
	}

	if len(children) == 0 {
		return false, nil, time.Time{}
	}

	sort.Strings(children)
	return true, children, latestMod
}

// --- S3 文件实现 ---

// s3File 实现了 File 接口，代表 S3 上的一个文件
type s3File struct {
	fs           *s3FS             // 所属文件系统
	name         string            // 规范化后的路径
	key          string            // S3 对象键
	fi           *ExtendedFileInfo // 文件元数据（缓存）
	metadataOnly bool              // 是否仅元数据模式

	mu     sync.Once     // 确保 MakeReadable 只执行一次
	reader io.ReadCloser // 文件内容读取器（延迟打开）
	err    error         // 打开过程中的错误
}

// 编译时检查 s3File 是否实现了 File 接口
var _ File = &s3File{}

// MakeReadable 将元数据模式的文件切换为可读模式
// 实际调用 GetObject 下载文件内容，使用 sync.Once 保证只执行一次
func (f *s3File) MakeReadable() error {
	if f.reader != nil {
		return nil
	}
	f.mu.Do(func() {
		ctx := context.Background()
		obj, err := f.fs.client.GetObject(ctx, f.fs.bucket, f.key, minio.GetObjectOptions{})
		if err != nil {
			f.err = err
			return
		}
		f.reader = obj
	})
	return f.err
}

// Read 读取文件内容，如果尚未打开会自动调用 MakeReadable
func (f *s3File) Read(p []byte) (int, error) {
	if f.reader == nil {
		if err := f.MakeReadable(); err != nil {
			return 0, pathError("read", f.name, err)
		}
	}
	return f.reader.Read(p)
}

// Close 关闭文件内容读取器
func (f *s3File) Close() error {
	if f.reader != nil {
		return f.reader.Close()
	}
	return nil
}

// Readdirnames 文件不支持读取目录条目，返回错误
func (f *s3File) Readdirnames(_ int) ([]string, error) {
	return nil, pathError("readdirnames", f.name, fmt.Errorf("not a directory"))
}

// Stat 返回文件的元数据信息
func (f *s3File) Stat() (*ExtendedFileInfo, error) {
	return f.fi, nil
}

// ToNode 将文件转换为 restic 的 Node 结构，用于快照存储
func (f *s3File) ToNode(_ bool, _ func(format string, args ...any)) (*data.Node, error) {
	node := buildBasicNode(f.name, f.fi)
	node.UID = f.fi.UID
	node.GID = f.fi.GID
	node.User = lookupUsername(f.fi.UID)
	node.Group = lookupGroup(f.fi.GID)
	node.ChangeTime = f.fi.ChangeTime
	node.AccessTime = f.fi.AccessTime
	node.Links = 1
	return node, nil
}

// --- S3 目录实现 ---

// s3Dir 实现了 File 接口，代表 S3 上的一个虚拟目录
type s3Dir struct {
	name     string            // 目录路径
	fi       *ExtendedFileInfo // 目录元数据
	entries  []string          // 子条目名称列表
	metadata bool              // 是否为元数据模式
}

// 编译时检查 s3Dir 是否实现了 File 接口
var _ File = &s3Dir{}

// MakeReadable 目录无需额外操作，直接返回成功
func (d *s3Dir) MakeReadable() error {
	return nil
}

// Read 目录不支持读取内容，返回错误
func (d *s3Dir) Read(_ []byte) (int, error) {
	return 0, pathError("read", d.name, fmt.Errorf("is a directory"))
}

// Close 目录无需关闭操作
func (d *s3Dir) Close() error {
	return nil
}

// Readdirnames 返回目录下的子条目名称列表
// n <= 0 时返回所有条目；n > 0 暂不支持
func (d *s3Dir) Readdirnames(n int) ([]string, error) {
	if n > 0 {
		return nil, pathError("readdirnames", d.name, fmt.Errorf("not implemented"))
	}
	return slices.Clone(d.entries), nil
}

// Stat 返回目录的元数据信息
func (d *s3Dir) Stat() (*ExtendedFileInfo, error) {
	return d.fi, nil
}

// ToNode 将目录转换为 restic 的 Node 结构，用于快照存储
func (d *s3Dir) ToNode(_ bool, _ func(format string, args ...any)) (*data.Node, error) {
	node := buildBasicNode(d.name, d.fi)
	node.UID = d.fi.UID
	node.GID = d.fi.GID
	node.User = lookupUsername(d.fi.UID)
	node.Group = lookupGroup(d.fi.GID)
	node.ChangeTime = d.fi.ChangeTime
	node.AccessTime = d.fi.AccessTime
	return node, nil
}
