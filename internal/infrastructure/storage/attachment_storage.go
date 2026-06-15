package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"finarch/internal/domain/service"
)

// LocalAttachmentStorage stores attachment bytes on the local filesystem.
type LocalAttachmentStorage struct {
	root string
}

// NewLocalAttachmentStorage creates a local attachment storage rooted under /data.
func NewLocalAttachmentStorage(root string) (*LocalAttachmentStorage, error) {
	if strings.TrimSpace(root) == "" {
		root = "/data/attachments"
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve attachment dir: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create attachment dir: %w", err)
	}
	return &LocalAttachmentStorage{root: abs}, nil
}

func (s *LocalAttachmentStorage) Save(ctx context.Context, userID, attachmentID, filename, declaredContentType string, r io.Reader, maxBytes int64) (service.StoredAttachment, error) {
	if r == nil {
		return service.StoredAttachment{}, fmt.Errorf("请上传文件")
	}
	if maxBytes <= 0 {
		maxBytes = service.DefaultAttachmentMaxBytes
	}
	ext := strings.ToLower(filepath.Ext(filename))
	if !allowedAttachmentExt(ext) {
		return service.StoredAttachment{}, fmt.Errorf("仅支持 JPG、PNG、WEBP 或 PDF 文件")
	}
	storageKey := filepath.Join(s.safeSegment(userID), attachmentID+ext)
	finalPath, ok := s.pathForKey(storageKey)
	if !ok {
		return service.StoredAttachment{}, fmt.Errorf("附件路径无效")
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
		return service.StoredAttachment{}, fmt.Errorf("创建附件目录失败: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(finalPath), ".upload-*")
	if err != nil {
		return service.StoredAttachment{}, fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	limited := &limitReader{r: r, max: maxBytes + 1}
	mw := io.MultiWriter(tmp, h)
	written, err := io.Copy(mw, limited)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return service.StoredAttachment{}, fmt.Errorf("保存附件失败: %w", err)
	}
	if written == 0 {
		return service.StoredAttachment{}, fmt.Errorf("附件为空")
	}
	if written > maxBytes || limited.tooLarge {
		return service.StoredAttachment{}, fmt.Errorf("附件过大，最大 %.0f MB", float64(maxBytes)/(1<<20))
	}
	contentType, err := sniffFile(tmpPath, ext)
	if err != nil {
		return service.StoredAttachment{}, err
	}
	if declaredContentType != "" && !compatibleContentType(declaredContentType, contentType) {
		return service.StoredAttachment{}, fmt.Errorf("文件类型与内容不匹配")
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return service.StoredAttachment{}, fmt.Errorf("设置附件权限失败: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		return service.StoredAttachment{}, fmt.Errorf("保存附件失败: %w", err)
	}
	select {
	case <-ctx.Done():
		_ = os.Remove(finalPath)
		return service.StoredAttachment{}, ctx.Err()
	default:
	}
	return service.StoredAttachment{StorageKey: filepath.ToSlash(storageKey), ContentType: contentType, SizeBytes: written, SHA256: hex.EncodeToString(h.Sum(nil))}, nil
}

func (s *LocalAttachmentStorage) Restore(ctx context.Context, storageKey string, r io.Reader) error {
	path, ok := s.pathForKey(storageKey)
	if !ok {
		return fmt.Errorf("invalid attachment key")
	}
	if r == nil {
		return fmt.Errorf("attachment reader is nil")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".restore-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	_, copyErr := io.Copy(tmp, r)
	closeErr := tmp.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func (s *LocalAttachmentStorage) Open(ctx context.Context, storageKey string) (io.ReadCloser, error) {
	path, ok := s.pathForKey(storageKey)
	if !ok {
		return nil, fmt.Errorf("invalid attachment key")
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	return os.Open(path)
}

func (s *LocalAttachmentStorage) Delete(ctx context.Context, storageKey string) error {
	path, ok := s.pathForKey(storageKey)
	if !ok {
		return fmt.Errorf("invalid attachment key")
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *LocalAttachmentStorage) pathForKey(key string) (string, bool) {
	cleaned := filepath.Clean(strings.TrimPrefix(filepath.FromSlash(key), string(os.PathSeparator)))
	if cleaned == "." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) || cleaned == ".." || filepath.IsAbs(cleaned) {
		return "", false
	}
	joined := filepath.Join(s.root, cleaned)
	abs, err := filepath.Abs(joined)
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(s.root, abs)
	if err != nil || filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", false
	}
	return abs, true
}

func (s *LocalAttachmentStorage) safeSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

type limitReader struct {
	r        io.Reader
	max      int64
	read     int64
	tooLarge bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.read >= l.max {
		l.tooLarge = true
		return 0, io.EOF
	}
	remaining := l.max - l.read
	if int64(len(p)) > remaining {
		p = p[:int(remaining)]
	}
	n, err := l.r.Read(p)
	l.read += int64(n)
	if l.read >= l.max {
		l.tooLarge = true
	}
	return n, err
}

func sniffFile(path, ext string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("读取附件失败: %w", err)
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := io.ReadFull(f, buf)
	if n == 0 {
		return "", fmt.Errorf("附件为空")
	}
	buf = buf[:n]
	contentType := http.DetectContentType(buf)
	if ext == ".pdf" {
		if len(buf) >= 4 && string(buf[:4]) == "%PDF" {
			return "application/pdf", nil
		}
		return "", fmt.Errorf("文件类型与 PDF 扩展名不匹配")
	}
	if ext == ".webp" {
		if len(buf) >= 12 && string(buf[:4]) == "RIFF" && string(buf[8:12]) == "WEBP" {
			return "image/webp", nil
		}
		return "", fmt.Errorf("文件类型与 WEBP 扩展名不匹配")
	}
	if ext == ".jpg" || ext == ".jpeg" {
		if contentType == "image/jpeg" {
			return contentType, nil
		}
		return "", fmt.Errorf("文件类型与 JPG 扩展名不匹配")
	}
	if ext == ".png" {
		if contentType == "image/png" {
			return contentType, nil
		}
		return "", fmt.Errorf("文件类型与 PNG 扩展名不匹配")
	}
	return "", fmt.Errorf("不支持的附件类型")
}

func allowedAttachmentExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".pdf":
		return true
	default:
		return false
	}
}

func compatibleContentType(declared, detected string) bool {
	declared = strings.ToLower(strings.TrimSpace(strings.Split(declared, ";")[0]))
	if declared == "" || declared == "application/octet-stream" {
		return true
	}
	if declared == detected {
		return true
	}
	return strings.HasPrefix(declared, "image/") && strings.HasPrefix(detected, "image/")
}
