package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"
)

func (cfg apiConfig) ensureAssetsDir() error {
	if _, err := os.Stat(cfg.assetsRoot); os.IsNotExist(err) {
		return os.Mkdir(cfg.assetsRoot, 0755)
	}
	return nil
}

func getAssetPath(mediaType string) string {
	ext := mediaTypeToExt(mediaType)
	key := make([]byte, 32)
	rand.Read(key)
	name := hex.EncodeToString(key)
	return fmt.Sprintf("%s%s", name, ext)
}

func (cfg apiConfig) getAssetSavePath(assetPath string) string {
	return filepath.Join(cfg.assetsRoot, assetPath)
}

func (cfg apiConfig) getObjectURL(key string) string {
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.s3Bucket, cfg.s3Region, key)
}

func (cfg apiConfig) getAssetURL(assetPath string) string {
	return fmt.Sprintf("http://localhost:%s/assets/%s", cfg.port, assetPath)
}

func mediaTypeToExt(mediaType string) string {
	parts := strings.Split(mediaType, "/")
	if len(parts) != 2 {
		return ".bin"
	}
	return "." + parts[1]
}


type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width,omitempty"`
		Height int `json:"height,omitempty"`
	}
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)

	var b bytes.Buffer
	cmd.Stdout = &b


	err := cmd.Run()
	if err != nil {
		return "", err
	}

	var output ffprobeOutput
	if err := json.Unmarshal(b.Bytes(), &output); err != nil {
	    return "", err
	} 


	w, h := output.Streams[0].Width, output.Streams[0].Height
	if 9*w/16 == h {
		return "16:9", nil
	} else if 16*w/9 == h {
		return "9:16", nil
	} 
	return "other", nil
} 


func processVideoForFastStart(filePath string) (string, error) {
	output := filePath + ".processing"
	return output, exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", output).Run()
} 

func generatePresignedURL(s3Client *s3.Client, bucket, key string, expireTime time.Duration) (string, error) {
	presignClient := s3.NewPresignClient(s3Client, s3.WithPresignExpires(expireTime))
	req, err := presignClient.PresignGetObject(context.TODO(), &s3.GetObjectInput{
		Bucket: &bucket,
		Key: &key,
	})
	if err != nil {
	    return "", err
	} 
	return req.URL, nil
} 

func (cfg *apiConfig) dbVideoToSignedVideo(video database.Video) (database.Video, error) {
	if video.VideoURL == nil {
	    return video, nil
	} 
	splitURL := strings.Split(*video.VideoURL, ",")
	bucket, key := splitURL[0], splitURL[1]

	newURL, err := generatePresignedURL(cfg.s3Client, bucket, key, 10 * time.Minute)
	if err != nil {
	    return database.Video{}, err
	} 
	video.VideoURL = &newURL
	return video, nil
} 
