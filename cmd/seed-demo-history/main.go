package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/oklog/ulid/v2"
)

type demoLink struct {
	ID       string
	DomainID string
	Slug     string
}

type clickProfile struct {
	Country  string
	Device   string
	OS       string
	Browser  string
	Language string
	Referrer string
	IsBot    bool
}

var clickProfiles = []clickProfile{
	{
		Country: "CN", Device: "desktop", OS: "Windows", Browser: "Chrome",
		Language: "zh-cn", Referrer: "https://search.example.com/result",
	},
	{
		Country: "SG", Device: "mobile", OS: "Android", Browser: "Chrome",
		Language: "en-sg", Referrer: "https://social.example.com/feed",
	},
	{
		Country: "JP", Device: "mobile", OS: "iOS", Browser: "Safari",
		Language: "ja-jp", Referrer: "https://newsletter.example.com/archive",
	},
	{
		Country: "DE", Device: "desktop", OS: "macOS", Browser: "Safari",
		Language: "de-de", Referrer: "",
	},
	{
		Country: "US", Device: "desktop", OS: "Linux", Browser: "Firefox",
		Language: "en-us", Referrer: "https://docs.example.com/integrations",
	},
	{
		Country: "", Device: "desktop", OS: "unknown", Browser: "unknown",
		Language: "", Referrer: "", IsBot: true,
	},
}

// main 为已有演示账号的链接生成分布在最近若干天的确定性点击数据。
func main() {
	email := flag.String("email", "demo@example.com", "演示账号邮箱")
	days := flag.Int("days", 7, "生成最近多少天的数据")
	seedName := flag.String("seed", "shorturl-demo-v1", "确定性数据种子")
	reset := flag.Bool("reset", false, "写入前清空该演示工作空间的点击事件")
	flag.Parse()

	if *days < 1 || *days > 30 {
		fmt.Fprintln(os.Stderr, "days 必须为 1 至 30")
		os.Exit(2)
	}
	dsn := strings.TrimSpace(os.Getenv("SHORTURL_MYSQL_DSN"))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "未配置 SHORTURL_MYSQL_DSN")
		os.Exit(2)
	}
	if err := run(
		context.Background(),
		dsn,
		strings.TrimSpace(*email),
		*days,
		*seedName,
		*reset,
	); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 查询演示工作空间和短链，并在单个事务中幂等写入历史点击。
func run(
	ctx context.Context,
	dsn, email string,
	days int,
	seedName string,
	reset bool,
) error {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("打开 MySQL 失败: %w", err)
	}
	defer func() {
		_ = db.Close()
	}()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	var workspaceID string
	if err := db.QueryRowContext(ctx, `
		SELECT w.id
		FROM users AS u
		INNER JOIN workspaces AS w ON w.owner_user_id = u.id
		WHERE u.email = ?
		LIMIT 1`, email).Scan(&workspaceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("未找到演示账号 %s 对应的工作空间", email)
		}
		return fmt.Errorf("查询演示工作空间失败: %w", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, domain_id, slug
		FROM links
		WHERE workspace_id = ? AND deleted_at IS NULL
		ORDER BY created_at ASC, id ASC`, workspaceID)
	if err != nil {
		return fmt.Errorf("查询演示短链失败: %w", err)
	}
	links := make([]demoLink, 0)
	for rows.Next() {
		var item demoLink
		if err := rows.Scan(&item.ID, &item.DomainID, &item.Slug); err != nil {
			_ = rows.Close()
			return fmt.Errorf("读取演示短链失败: %w", err)
		}
		links = append(links, item)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭演示短链游标失败: %w", err)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("读取演示短链失败: %w", err)
	}
	if len(links) == 0 {
		return fmt.Errorf("演示账号 %s 没有可用短链", email)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("开始演示数据事务失败: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	if reset {
		if _, err := tx.ExecContext(
			ctx,
			`DELETE FROM click_events WHERE workspace_id = ?`,
			workspaceID,
		); err != nil {
			return fmt.Errorf("清理演示点击失败: %w", err)
		}
	}
	inserted := int64(0)
	now := time.Now().UTC()
	for linkIndex, item := range links {
		for dayIndex := 0; dayIndex < days; dayIndex++ {
			dayKey := now.Truncate(24*time.Hour).
				AddDate(0, 0, -(days - dayIndex)).
				Format("2006-01-02")
			daySeed := digest(seedName, item.ID, dayKey, 0, 0)
			count := 5 + int(daySeed[1]%6)
			for sampleIndex := 0; sampleIndex < count; sampleIndex++ {
				timestamp := eventTimestamp(now, days, dayIndex, linkIndex, sampleIndex)
				eventSeed := digest(seedName, item.ID, dayKey, sampleIndex, 1)
				profile := clickProfiles[int(eventSeed[0])%len(clickProfiles)]
				visitorHash := digest(seedName, fmt.Sprintf("visitor-%d", int(eventSeed[2])%48))
				eventID, err := deterministicULID(timestamp, eventSeed)
				if err != nil {
					return fmt.Errorf("生成演示事件 ID 失败: %w", err)
				}
				result, err := tx.ExecContext(ctx, `
					INSERT IGNORE INTO click_events (
						event_id, workspace_id, link_id, domain_id, slug, clicked_at,
						visitor_hash, country, device, os, browser, language, referrer,
						utm_source, utm_medium, utm_campaign, utm_term, utm_content,
						is_bot, created_at
					) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					eventID, workspaceID, item.ID, item.DomainID, item.Slug, timestamp,
					visitorHash[:], profile.Country, profile.Device, profile.OS,
					profile.Browser, profile.Language, profile.Referrer,
					utmSource(profile.Referrer), "", "", "", "", profile.IsBot, timestamp,
				)
				if err != nil {
					return fmt.Errorf("写入演示点击失败: %w", err)
				}
				affected, err := result.RowsAffected()
				if err != nil {
					return fmt.Errorf("读取演示点击写入行数失败: %w", err)
				}
				inserted += affected
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交演示点击失败: %w", err)
	}
	fmt.Printf(
		"演示历史数据完成: email=%s links=%d days=%d inserted=%d\n",
		email,
		len(links),
		days,
		inserted,
	)
	return nil
}

// digest 生成与输入顺序绑定、可重复的摘要，用于演示数据和事件 ID。
func digest(parts ...any) [32]byte {
	text := make([]string, 0, len(parts))
	for _, part := range parts {
		text = append(text, fmt.Sprint(part))
	}
	return sha256.Sum256([]byte(strings.Join(text, "|")))
}

// eventTimestamp 将事件分布到每天的访问时段，并保持稳定顺序。
func eventTimestamp(
	now time.Time,
	days, dayIndex, linkIndex, sampleIndex int,
) time.Time {
	day := now.Truncate(24*time.Hour).AddDate(0, 0, -(days - dayIndex))
	hour := 8 + (sampleIndex*3+linkIndex*2+dayIndex)%13
	minute := (sampleIndex*11 + linkIndex*7 + dayIndex*13) % 60
	return day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
}

// deterministicULID 根据时间与摘要生成稳定的 ULID。
func deterministicULID(timestamp time.Time, entropy [32]byte) (string, error) {
	id, err := ulid.New(uint64(timestamp.UnixMilli()), strings.NewReader(string(entropy[:])))
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// utmSource 为部分来源流量生成稳定的 UTM 来源标签。
func utmSource(referrer string) string {
	if referrer == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(referrer))
	if binary.BigEndian.Uint32(sum[:4])%3 != 0 {
		return ""
	}
	return "demo-newsletter"
}
