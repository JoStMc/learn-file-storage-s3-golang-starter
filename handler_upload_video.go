package main

import (
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadVideo(w http.ResponseWriter, r *http.Request) {
	const uploadLimit = 1 << 30
	r.Body = http.MaxBytesReader(w, r.Body, uploadLimit)

	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	video, err:= cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to get video", err)
		return
	}
	if userID != video.UserID {
		respondWithError(w, http.StatusUnauthorized, "Unathorized", err)
		return
	} 

	file, header, err := r.FormFile("video")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Error getting multipart file", err)
		return
	}
	defer file.Close()

	mediatype, _, err := mime.ParseMediaType(header.Header.Get("Content-Type"))
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse media type", err)
		return
	}
	if mediatype != "video/mp4" {
	    respondWithError(w, http.StatusBadRequest, "Invalid mediatype", nil)
		return
	} 

	tempFile, err := os.CreateTemp("", "tubely-upload.mp4")
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to create file", err)
		return
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, file)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to copy video data", err)
		return
	}

	aspectRatio, err := getVideoAspectRatio(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video aspect ratio", err)
		return
	}

	var prefix string
	switch aspectRatio {
		case "16:9":
			prefix = "landscape/"
		case "9:16":
			prefix = "portrait/"
		default:
			prefix = "other/"
	}

	_, err = tempFile.Seek(0, io.SeekStart)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to reset temp file's pointer", err)
		return
	}
	processedVideoName, err := processVideoForFastStart(tempFile.Name())
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to process video", err)
		return
	}
	processedVideo, err := os.Open(processedVideoName)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to open processed video", err)
		return
	}
	defer os.Remove(processedVideo.Name())
	defer processedVideo.Close()

	key := prefix + getAssetPath(mediatype)
	_, err = cfg.s3Client.PutObject(r.Context(), &s3.PutObjectInput{
		Bucket: aws.String(cfg.s3Bucket),
		Key: aws.String(key),
		Body: processedVideo,
		ContentType: aws.String(mediatype),
	})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to put S3 object", err)
		return
	}


	// videoURL := cfg.getObjectURL(key)
	videoURL := cfg.s3Bucket + "," + key
	video.VideoURL = &videoURL
	err = cfg.db.UpdateVideo(video)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "Unable to update video", err)
		return
	} 
	video, err = cfg.dbVideoToSignedVideo(video)
	if err != nil {
	    respondWithError(w, http.StatusInternalServerError, "Unable to get signed video", err)
		return
	} 
	respondWithJSON(w, http.StatusOK, video)
}
