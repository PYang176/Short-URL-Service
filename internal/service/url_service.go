package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"shortURL/internal/model"
	"shortURL/internal/pool"
	"shortURL/internal/redis"
	"shortURL/internal/repository"
	"shortURL/internal/task"
	"shortURL/pkg/base62"
	"strings"
	"time"
)

type URLService struct {
	repo     *repository.URLRepo
	taskPool *pool.Pool // 协程池
}

func NewURLService(repo *repository.URLRepo, pool *pool.Pool) *URLService {
	return &URLService{repo: repo, taskPool: pool}
}

// Create 创建短链
func (s *URLService) Create(req *model.CreateRequest) (string, error) {
	ctx := context.Background()

	// 自动补协议头
	originalURL := req.OriginalURL
	if !strings.HasPrefix(originalURL, "http://") && !strings.HasPrefix(originalURL, "https://") {
		originalURL = "http://" + originalURL
	}
	// 如果是自定义短码，或者强制刷新，就不查库直接走创建流程
	if req.CustomCode == "" && !req.ForceRefresh {
		existingURL, err := s.repo.FindByOriginalURL(ctx, originalURL)
		if err == nil && existingURL != nil {
			// 检查是否过期
			now := time.Now().Unix()
			if existingURL.ExpiredAt == 0 || existingURL.ExpiredAt > now {
				log.Printf("[INFO] Found existing active short code for %s: %s", originalURL, existingURL.ShortCode)
				return existingURL.ShortCode, nil // 返回旧的短码
			}
			log.Printf("[INFO] Found expired short code for %s, will regenerate", originalURL)
		}
	}
	// 如果 existingURL 过期了，生成新短码

	var shortCode string

	// 判断是自定义还是随机
	if req.CustomCode != "" {
		exists, err := s.repo.Exists(ctx, req.CustomCode)
		if err != nil {
			return "", err
		}
		if exists {
			return "", errors.New("custom code already exists")
		}
		shortCode = req.CustomCode
	} else {
		// 随机生成
		for {
			code, err := generateRandomCode(6)
			if err != nil {
				return "", err
			}
			exists, _ := s.repo.Exists(ctx, code)
			if !exists {
				shortCode = code
				break
			}
		}
	}

	// 构建 Model
	url := &model.URL{
		OriginalURL: req.OriginalURL,
		ShortCode:   shortCode,
		IsCustom:    req.CustomCode != "",
		Clicks:      0,
		CreatedAt:   time.Now().Unix(),
		ExpiredAt:   req.ExpiredAt,
	}

	// 存入数据库
	_, err := s.repo.Insert(ctx, url)
	if err != nil {
		return "", err
	}

	return shortCode, nil
}

// 生成随机字符串
func generateRandomCode(length int) (string, error) {
	result := make([]byte, length)
	for i := range result {
		num, err := rand.Int(rand.Reader, big.NewInt(62))
		if err != nil {
			return "", err
		}
		result[i] = base62.Charset[num.Int64()]
	}
	return string(result), nil
}

// GetAndIncrement 获取原始 URL 并增加点击量
func (s *URLService) GetAndIncrement(code string) (string, error) {
	ctx := context.Background()
	redisKey := "short:url:" + code

	// 先从Redis查缓存
	originalURL, err := redis.RDB.Get(redis.Ctx, redisKey).Result()
	if err == nil {
		// 缓存命中
		if originalURL == "" {
			// 命中空值缓存，直接返回防击穿
			log.Printf("[REDIS] Hit null cache for %s", code)
			return "", errors.New("not found")
		}
		log.Printf("[REDIS] Hit cache for %s", code)
		return originalURL, nil
	} else if err != nil {
		// Redis 挂了，记录日志，降级到 DB
		log.Printf("[ERROR] Redis error: %v, fallback to DB", err)
	}

	// 数据库查询短链
	url, err := s.repo.FindByCode(ctx, code)
	fmt.Println(url)
	if err != nil {
		log.Printf("Error finding URL for code %s: %v", code, err)
		return "", errors.New("not found")
	}
	if url == nil {
		// 写入Redis空值
		redis.RDB.Set(redis.Ctx, redisKey, "", 5*time.Minute)
		log.Printf("URL not found for code %s", code)
		return "", errors.New("not found")
	}

	// 检查是否过期
	now := time.Now().Unix()
	if url.ExpiredAt != 0 && url.ExpiredAt <= now {
		log.Printf("URL expired for code %s: expired_at=%d, now=%d", code, url.ExpiredAt, now)
		return "", errors.New("expired")
	}

	// 未过期数据库有，回写 Redis
	redis.RDB.Set(redis.Ctx, redisKey, url.OriginalURL, 24*time.Hour)

	// 提交异步任务到协程池
	incrementTask := &task.IncrementTask{
		Repo: *s.repo,
		Code: code,
		Ctx:  ctx,
	}
	s.taskPool.Submit(incrementTask)

	return url.OriginalURL, nil
}
