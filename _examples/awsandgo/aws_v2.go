package main

import (
	"bytes"
	"context"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
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
	if err := cacheInit(); err != nil {
		log.Fatal(err)
	}

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
	var (
		engine = gin.Default()
		server = http.Server{Addr: ":8080", Handler: engine}
	)
	engine.POST("/upload", uploadHandler)
	engine.POST("/upload/:key", uploadHandler)
	engine.GET("/download/:key", downloadHandler)
	go func() {
		if err := server.ListenAndServe(); err != nil {
			log.Fatalln("Startup:", err)
		}
	}()

	quit := make(chan os.Signal)
	signal.Notify(quit, os.Interrupt)
	<-quit

	ctx, exit := context.WithTimeout(context.Background(), 60*time.Second)
	defer exit()
	if err := cacheClean(); err != nil {
		log.Println("Clean:", err)
	}
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalln("Shutdown:", err)
	}
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
	path, err := cacheEnsure(key)
	if err != nil {
		log.Printf("%s: get error (%s)", key, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	http.ServeFile(c.Writer, c.Request, path)
}

const keep = time.Hour

var cache struct {
	sync.Mutex
	timers map[string]*time.Timer
	root   string
	client *s3.S3
}

func cacheInit() error {
	cache.Lock()
	cache.timers = make(map[string]*time.Timer)
	cache.root = os.TempDir()
	cache.Unlock()
	return os.MkdirAll(cache.root, 0644)
}
func cacheClean() error { return os.RemoveAll(cache.root) }

func cacheEnsure(key string) (string, error) {
	cache.Lock()
	t := cache.timers[key]
	cache.Unlock()
	if t == nil {
		if err := cacheDownload(key); err != nil {
			return "", err
		}
	}
	cache.Lock()
	cache.timers[key].Reset(keep)
	cache.Unlock()
	return cachePath(key), nil
}

func cachePath(key string) string { return filepath.Join(cache.root, key) }

func cacheDownload(key string) error {
	src, err := client.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(Bucket),
		Key:    aws.String(key),
	})
	defer src.Body.Close()

	dst, err := os.OpenFile(cachePath(key), os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src.Body); err != nil {
		return err
	}

	cache.Lock()
	cache.timers[key] = cacheDeleteTimer(key)
	cache.Unlock()
	return nil
}

func cacheDeleteTimer(key string) *time.Timer {
	return time.AfterFunc(keep, func() {
		cache.Lock()
		delete(cache.timers, key)
		os.Remove(cachePath(key))
		cache.Unlock()
	})
}
