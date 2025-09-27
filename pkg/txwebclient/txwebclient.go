package txwebclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type File struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"mod_time"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (lr *LoginRequest) toJSON() []byte {
	b, _ := json.Marshal(lr)
	return b
}

type LoginResponse struct {
	Message string `json:"message"`
	Token   string `json:"token"`
	User    string `json:"user"`
	Expires int64  `json:"expires"`
}

func Login(endpoint, username, password string) (string, int64, error) {
	req := &LoginRequest{
		Username: username,
		Password: password,
	}
	r := bytes.NewReader(req.toJSON())

	resp, err := http.Post(endpoint+"/login", "application/json", r)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return "", 0, fmt.Errorf("too many login attempts, please wait and try again later")
	case http.StatusUnauthorized:
		return "", 0, fmt.Errorf("invalid username or password")
	default:
		if resp.StatusCode != http.StatusOK {
			return "", 0, fmt.Errorf("login failed (%d)", resp.StatusCode)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	var loginResp LoginResponse
	if err := json.Unmarshal(body, &loginResp); err != nil {
		return "", 0, err
	}

	return loginResp.Token, loginResp.Expires, nil
}

type Client struct {
	endpoint string
	token    string
	client   *http.Client
}

type ClientOptions func(*Client)

func WithEndpoint(endpoint string) ClientOptions {
	return func(c *Client) {
		c.endpoint = endpoint
	}
}

func WithToken(token string) ClientOptions {
	return func(c *Client) {
		c.token = token
	}
}

func New(opts ...ClientOptions) *Client {
	c := &Client{
		endpoint: "https://api.txlogger.com",
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) SetToken(token string) {
	c.token = token
}

func (c *Client) SetEndpoint(endpoint string) {
	c.endpoint = endpoint
}

func (c *Client) DeleteFile(filename string) error {
	if c.token == "" {
		return ErrMissingToken
	}
	req, err := http.NewRequest("DELETE", c.endpoint+"/api/files/"+filename, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete file (%d)", resp.StatusCode)
	}

	return nil
}

func (c *Client) ListFiles() ([]File, error) {
	if c.token == "" {
		return nil, ErrMissingToken
	}
	req, err := http.NewRequest("GET", c.endpoint+"/api/files", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Add("Authorization", "Bearer "+c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list files (%d)", resp.StatusCode)
	}

	var files []File
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}

	return files, nil
}

func (c *Client) DownloadFile(filename string) ([]byte, error) {
	if c.token == "" {
		return nil, ErrMissingToken
	}
	req, err := http.NewRequest("GET", c.endpoint+"/api/files/"+filename, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download file (%d)", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return data, nil
}

/*
func (c *Client) UploadFile(filename string) error {
	if c.token == "" {
		return ErrMissingToken
	}
	fh, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer fh.Close()

	// Prepare multipart body
	pr, pw := io.Pipe()

	writer := multipart.NewWriter(pw)

	// Stream the file into the multipart writer in a goroutine
	go func() {
		defer pw.Close()
		defer writer.Close()

		partName := filepath.Base(filename)
		//if *name != "" {
		//	partName = *name
		//}

		part, err := writer.CreateFormFile("file", partName)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		// Simple progress indicator
		var uploaded int64
		buf := make([]byte, 256*1024)
		lastPrint := time.Now()
		for {
			n, rerr := fh.Read(buf)
			if n > 0 {
				if _, werr := part.Write(buf[:n]); werr != nil {
					_ = pw.CloseWithError(werr)
					return
				}
				uploaded += int64(n)
				if time.Since(lastPrint) > 750*time.Millisecond {
					log.Printf("\ruploaded %d bytes...", uploaded)
					lastPrint = time.Now()
				}
			}
			if rerr == io.EOF {
				break
			}
			if rerr != nil {
				_ = pw.CloseWithError(rerr)
				return
			}
		}

		// Optional extra field for server-side naming (server still sanitizes)
		//if *name != "" {
		//	_ = writer.WriteField("name", *name)
		//}
	}()

	req, err := http.NewRequest("POST", c.endpoint+"/api/upload", pr)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Add("Authorization", "Bearer "+c.token)

	// Timeout & context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("server responded: %s\n", resp.Status)
	body, _ := io.ReadAll(resp.Body)
	log.Println(string(body))
	return nil
}
*/

// UploadFile reads a local path and uploads it.
func (c *Client) UploadFile(path string) error {
	if c.token == "" {
		return ErrMissingToken
	}
	fh, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer fh.Close()

	return c.uploadMultipart(filepath.Base(path), fh)
}

// UploadFileBytes uploads an in-memory buffer.
func (c *Client) UploadFileBytes(filename string, data []byte) error {
	if c.token == "" {
		return ErrMissingToken
	}
	return c.uploadMultipart(filepath.Base(filename), bytes.NewReader(data))
}

// UploadFileReader uploads from an arbitrary reader (e.g., file, network stream).
func (c *Client) UploadFileReader(filename string, r io.Reader) error {
	if c.token == "" {
		return ErrMissingToken
	}
	return c.uploadMultipart(filepath.Base(filename), r)
}

func (c *Client) uploadMultipart(partName string, src io.Reader) error {
	// Pipe lets us stream the multipart body without buffering it all in memory.
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)

	// Build body concurrently.
	go func() {
		defer func() {
			_ = writer.Close() // close multipart boundary
			_ = pw.Close()     // close pipe writer (signals EOF to request body)
		}()

		part, err := writer.CreateFormFile("file", partName)
		if err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		// Wrap the multipart part with a progress logger.
		pwProgress := &progressWriter{
			w:          part,
			lastPrint:  time.Now(),
			printEvery: 750 * time.Millisecond,
		}

		// Stream copy from src → multipart part
		if _, err := io.Copy(pwProgress, src); err != nil {
			_ = pw.CloseWithError(err)
			return
		}

		// If you want to add extra fields (e.g., an explicit name), do it before writer.Close():
		// _ = writer.WriteField("name", partName)
	}()

	req, err := http.NewRequest("POST", c.endpoint+"/api/upload", pr)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Add("Authorization", "Bearer "+c.token)

	// Timeout & context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("server responded: %s\n", resp.Status)
	body, _ := io.ReadAll(resp.Body)
	log.Println(string(body))
	return nil
}
