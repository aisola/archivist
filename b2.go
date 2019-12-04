package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/aisola/archivist/middleware"
)

var (
	errUnauthorized = errors.New("unauthorized")
	errTooManyRequests = errors.New("too many requests")
	errTimeout = errors.New("request timeout")
)

type B2 struct {
	logger *zap.Logger
	client *http.Client

	keyID    string
	keyToken string
	bucketID string

	apiURL   string
	apiToken string

	uploadAuthToken string
	uploadURL       string
}

func NewB2(logger *zap.Logger, keyID, keyToken, bucketID string) *B2 {
	return &B2{
		logger: logger,
		client: &http.Client{
			Transport: &http.Transport{
				IdleConnTimeout:     90 * time.Second,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 100,
				MaxConnsPerHost:     100,
			},
		},
		keyID:    keyID,
		keyToken: keyToken,
		bucketID: bucketID,
	}
}


func (b *B2) info(ctx context.Context, msg string, fields ...zap.Field) {
	if id := middleware.GetRequestID(ctx); id != "" {
		fields = append(fields, zap.String("request_id", id))
	}

	b.logger.Info(msg, fields...)
}

func (b *B2) error(ctx context.Context, msg string, fields ...zap.Field) {
	if id := middleware.GetRequestID(ctx); id != "" {
		fields = append(fields, zap.String("request_id", id))
	}

	b.logger.Error(msg, fields...)
}

func (b *B2) Upload(ctx context.Context, r io.Reader, fileName, fileMediaType, hash string, size int) (string, error) {
	var (
		fileID string
		succeeded bool
		lastError error
	)

	// Failures to upload are not uncommon so we do automatic retries here just to make it a bit more reliable.
	for i := 0; i < 5 && !succeeded; i++ {
		// Get new upload URL if necessary
		if b.uploadURL == "" || b.uploadAuthToken == "" {
			if err := b.getUploadURL(ctx); err != nil {
				return "", fmt.Errorf("could not get upload url: %w", err)
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.uploadURL, r)
		if err != nil {
			return "", fmt.Errorf("could not create request: %w", err)
		}
		req.Header.Set("Authorization", b.uploadAuthToken)
		req.Header.Set("X-Bz-File-Name", fileName)
		req.Header.Set("Content-Type", fileMediaType)
		req.Header.Set("Content-Length", fmt.Sprintf("%d", size))
		req.Header.Set("X-Bz-Content-Sha1", hash)

		response, err := b.client.Do(req)
		if err != nil {
			b.error(ctx, "failed to make request, probably some issue with a b2 node, setting to get new upload url next time",
				zap.Error(err),
			)
			b.uploadURL = ""
			b.uploadAuthToken = ""
			lastError = fmt.Errorf("failed to make request: %w", err)
			continue
		}

		switch response.StatusCode {
		case http.StatusOK:
			id, err := getFileID(response)
			if err != nil {
				return "", fmt.Errorf("failed to get id from response body: %w", err)
			}
			fileID = id
			lastError = nil
			succeeded = true
		case http.StatusUnauthorized:
			b.info(ctx, "failed to authorize, time for a new token")
			b.uploadURL = ""
			b.uploadAuthToken = ""
			lastError = errUnauthorized
		case http.StatusRequestTimeout:
			b.info(ctx, "request timed out, delaying and hoping for the best (ie, it goes faster next time)")
			lastError = errTimeout
			time.Sleep(1 * time.Second)
		case http.StatusTooManyRequests:
			b.info(ctx, "they're hammered, sleep it off, then retry")
			lastError = errTooManyRequests
			time.Sleep(1 * time.Second)
		default:
			data, err := ioutil.ReadAll(response.Body)
			if err != nil {
				data = []byte("could not readall from b2 body")
			}

			b.error(ctx, "we fucked up",
				zap.Int("b2_response_code", response.StatusCode),
				zap.ByteString("b2_response_body", data),
			)

			lastError = errors.New(string(data))
		}

		// Cannot defer this because it will only be called at the end of the function call. We're in a for loop.
		response.Body.Close()
	}

	if !succeeded {
		return "", lastError
	}

	return fileID, nil
}

func (b *B2) getUploadURL(ctx context.Context) error {
	if err := b.authorizeAccount(ctx); err != nil {
		return fmt.Errorf("failed to authorize account: %w", err)
	}

	const path = "%s/b2api/v2/b2_get_upload_url"

	data, err := json.Marshal(map[string]string{"bucketId": b.bucketID})
	if err != nil {
		panic("failed to marshal bucket id body, this is a BUG")
	}

	fmt.Printf("BUCKET INFO: %s\n", string(data))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fmt.Sprintf(path, b.apiURL), bytes.NewBuffer(data))
	if err != nil {
		return fmt.Errorf("failed to make get upload url request: %w", err)
	}
	req.Header.Set("Authorization", b.apiToken)

	response, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do get upload url request: %w", err)
	}
	defer response.Body.Close()

	data, err = ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read get upload url response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to get upload url: %w", errors.New(string(data)))
	}

	var o map[string]interface{}
	if err := json.Unmarshal(data, &o); err != nil {
		return fmt.Errorf("get upload url json unmarshalling error: %w", err)
	}

	b.uploadURL = o["uploadUrl"].(string)
	b.uploadAuthToken = o["authorizationToken"].(string)
	return nil
}

func (b *B2) authorizeAccount(ctx context.Context) error {
	const url = "https://api.backblazeb2.com/b2api/v2/b2_authorize_account"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to make authorize account request: %w", err)
	}
	req.SetBasicAuth(b.keyID, b.keyToken)

	response, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to do authorize account request: %w", err)
	}
	defer response.Body.Close()

	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("failed to read authorize account response body: %w", err)
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to authorize account account: %w", errors.New(string(data)))
	}

	var o map[string]interface{}
	if err := json.Unmarshal(data, &o); err != nil {
		return fmt.Errorf("authorize account json unmarshalling error: %w", err)
	}

	b.apiURL = o["apiUrl"].(string)
	b.apiToken = o["authorizationToken"].(string)
	return nil
}

func getFileID(response *http.Response) (string, error) {
	data, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read upload response body: %w", err)
	}

	var o map[string]interface{}
	if err := json.Unmarshal(data, &o); err != nil {
		return "", fmt.Errorf("upload json unmarshalling error: %w", err)
	}

	return o["fileId"].(string), nil
}
