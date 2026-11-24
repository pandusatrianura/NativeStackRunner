package runner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPClient abstracts sending HTTP requests for testability.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SignS3Request applies AWS Signature Version 4 headers to an S3 HTTP request.
func SignS3Request(req *http.Request, accessKey, secretKey, region string, now time.Time) {
	if region == "" {
		region = "us-east-1"
	}
	dateStamp := now.UTC().Format("20060102")
	amzDate := now.UTC().Format("20060102T150405Z")

	const emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	req.Header.Set("x-amz-date", amzDate)
	req.Header.Set("x-amz-content-sha256", emptyPayloadSHA256)

	host := req.URL.Host
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", host, emptyPayloadSHA256, amzDate)
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"

	canonicalReq := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		req.URL.Path,
		req.URL.RawQuery,
		canonicalHeaders,
		signedHeaders,
		emptyPayloadSHA256,
	)

	credScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, region)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credScope,
		sha256Hex([]byte(canonicalReq)),
	)

	kDate := hmacSHA256([]byte("AWS4"+secretKey), dateStamp)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, "s3")
	kSigning := hmacSHA256(kService, "aws4_request")

	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey, credScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

// EnsureMinIOBucket sends a signed S3 CreateBucket PUT request to MinIO.
func EnsureMinIOBucket(ctx context.Context, client HTTPClient, endpoint, accessKey, secretKey, bucket string) error {
	bucket = strings.TrimSpace(bucket)
	if bucket == "" {
		return nil
	}
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	cleanEndpoint := strings.TrimRight(endpoint, "/")
	reqURL := fmt.Sprintf("%s/%s", cleanEndpoint, bucket)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating bucket request: %w", err)
	}

	SignS3Request(req, accessKey, secretKey, "us-east-1", time.Now())

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("sending create bucket request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusConflict {
		// 200 OK = created, 409 Conflict = already exists
		return nil
	}

	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if strings.Contains(bodyStr, "BucketAlreadyOwnedByYou") || strings.Contains(bodyStr, "BucketAlreadyExists") {
		return nil
	}

	return fmt.Errorf("creating bucket %q returned HTTP %d: %s", bucket, resp.StatusCode, bodyStr)
}
