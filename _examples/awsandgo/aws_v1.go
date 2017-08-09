package main

import (
	"bytes"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/oklog/ulid"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

const (
	Region = endpoints.UsWest1RegionID
	Bucket = "video.player"
)

var client *s3.S3

func init() {
	client = s3.New(session.New(&aws.Config{Region: aws.String(Region)}))
	// ensure that the bucket exists
	buckets, err := client.ListBuckets(&s3.ListBucketsInput{})
	if err != nil {
		log.Fatal(err)
	}
	for _, b := range buckets.Buckets {
		if *b.Name == Bucket {
			return
		}
	}
	// create the bucket
	_, err = client.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(Bucket),
		ACL:    aws.String("bucket-owner-full-control"),
		CreateBucketConfiguration: &s3.CreateBucketConfiguration{
			LocationConstraint: aws.String(Region),
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Created", Bucket)
}

func main() {
	e := gin.Default()
	e.POST("/upload", uploadHandler)
	e.POST("/upload/:key", uploadHandler)
	e.GET("/download/:key", downloadHandler)
	e.Run(":8080")
}

var entropy = rand.New(rand.NewSource(time.Now().UnixNano()))

func uploadHandler(c *gin.Context) {
	var key = c.Param("key")
	if key == "" {
		key = ulid.MustNew(ulid.Timestamp(time.Now()), entropy).String()
	}
	b, err := ioutil.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("%s: upload error (%s)", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	reader := bytes.NewReader(b)
	_, err = client.PutObject(&s3.PutObjectInput{
		Bucket:        aws.String(Bucket),
		Key:           aws.String(key),
		Body:          reader,
		ContentLength: aws.Int64(reader.Size()),
	})
	if err != nil {
		log.Printf("%s: put error (%s)", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"key": key})
}

func downloadHandler(c *gin.Context) {
	var key = c.Param("key")

	obj, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		log.Printf("%s: get error (%s)", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	defer obj.Body.Close()

	_, err = io.Copy(c.Writer, obj.Body)
	if err != nil {
		log.Printf("%s: download error (%s)", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
}
