// CSV 导入服务 / CSV Import Service
// 功能：通用商品 CSV 的干跑预览、行级错误与幂等执行；商品批量写入与任务状态同事务提交
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-08-21 12:23:42
package importer

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/catalog"
)

const maxCSVBytes = 5 << 20

var requiredColumns = []string{"title", "slug", "status", "description", "option1_name", "option1_value", "option2_name", "option2_value", "sku", "price_cents", "quantity"}

type RowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}
type Preview struct {
	PublicID     string     `json:"public_id"`
	Status       string     `json:"status"`
	TotalRows    int        `json:"total_rows"`
	ProductCount int        `json:"product_count"`
	VariantCount int        `json:"variant_count"`
	Errors       []RowError `json:"errors"`
}
type Service struct {
	db      *sql.DB
	catalog *catalog.Service
}

func New(db *sql.DB, catalogSvc *catalog.Service) *Service {
	return &Service{db: db, catalog: catalogSvc}
}

// PreviewCSV 解析并持久化一次干跑。相同字节内容返回原任务，不会产生第二份任务。
func (s *Service) PreviewCSV(ctx context.Context, src io.Reader) (Preview, error) {
	b, err := io.ReadAll(io.LimitReader(src, maxCSVBytes+1))
	if err != nil {
		return Preview{}, fmt.Errorf("import: 读取 CSV: %w", err)
	}
	if len(b) > maxCSVBytes {
		return Preview{}, &ValidationError{Message: "CSV 不能超过 5MB"}
	}
	h := sha256.Sum256(b)
	hash := hex.EncodeToString(h[:])
	if old, err := s.getByHash(ctx, hash); err == nil {
		return old, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Preview{}, err
	}
	p, inputs := parseCSV(b)
	p.PublicID = uuid.Must(uuid.NewV7()).String()
	if len(p.Errors) == 0 && len(inputs) > 0 {
		p.Status = "previewed"
	} else {
		p.Status = "rejected"
	}
	errs, _ := json.Marshal(p.Errors)
	_, err = s.db.ExecContext(ctx, `INSERT INTO import_job (public_id, source_sha256, source_csv, status, total_rows, product_count, variant_count, errors_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, p.PublicID, hash, string(b), p.Status, p.TotalRows, p.ProductCount, p.VariantCount, string(errs))
	if err != nil {
		return Preview{}, fmt.Errorf("import: 保存预览: %w", err)
	}
	return p, nil
}

// Execute 将一个无错误预览一次性写入；成功任务重复执行只返回原结果。
func (s *Service) Execute(ctx context.Context, publicID string) (Preview, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Preview{}, fmt.Errorf("import: 开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	p, source, err := getByID(ctx, tx, publicID)
	if err != nil {
		return Preview{}, err
	}
	if p.Status == "succeeded" {
		return p, nil
	}
	if p.Status != "previewed" {
		return Preview{}, &ValidationError{Message: "该导入任务存在行错误，不能执行"}
	}
	check, inputs := parseCSV([]byte(source))
	if len(check.Errors) != 0 || len(inputs) == 0 {
		return Preview{}, &ValidationError{Message: "导入源已无效，请重新预览"}
	}
	if _, err := s.catalog.CreateProductsTx(ctx, tx, inputs); err != nil {
		return Preview{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE import_job SET status = 'succeeded', completed_at = strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE public_id = ? AND status = 'previewed'`, publicID); err != nil {
		return Preview{}, fmt.Errorf("import: 完成任务: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Preview{}, fmt.Errorf("import: 提交任务: %w", err)
	}
	p.Status = "succeeded"
	return p, nil
}

func (s *Service) Get(ctx context.Context, publicID string) (Preview, error) {
	p, _, err := getByID(ctx, s.db, publicID)
	return p, err
}
func (s *Service) getByHash(ctx context.Context, hash string) (Preview, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT public_id FROM import_job WHERE source_sha256=?`, hash).Scan(&id)
	if err != nil {
		return Preview{}, err
	}
	return s.Get(ctx, id)
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getByID(ctx context.Context, q queryer, id string) (Preview, string, error) {
	var p Preview
	var raw string
	var source string
	err := q.QueryRowContext(ctx, `SELECT public_id,status,total_rows,product_count,variant_count,errors_json,source_csv FROM import_job WHERE public_id=?`, id).Scan(&p.PublicID, &p.Status, &p.TotalRows, &p.ProductCount, &p.VariantCount, &raw, &source)
	if err != nil {
		return Preview{}, "", err
	}
	if err := json.Unmarshal([]byte(raw), &p.Errors); err != nil {
		return Preview{}, "", fmt.Errorf("import: 读错误报告: %w", err)
	}
	return p, source, nil
}

func parseCSV(b []byte) (Preview, []catalog.ProductInput) {
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	head, err := r.Read()
	if err != nil {
		return Preview{Errors: []RowError{{Row: 1, Message: "CSV 缺少表头"}}}, nil
	}
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.TrimSpace(h)] = i
	}
	for _, name := range requiredColumns {
		if _, ok := idx[name]; !ok {
			return Preview{Errors: []RowError{{Row: 1, Message: "缺少列 " + name}}}, nil
		}
	}
	type grouped struct {
		in       catalog.ProductInput
		variants []catalog.VariantInput
	}
	groups := map[string]*grouped{}
	order := []string{}
	p := Preview{}
	for line := 2; ; line++ {
		row, e := r.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "CSV 格式错误"})
			continue
		}
		p.TotalRows++
		get := func(k string) string {
			n := idx[k]
			if n >= len(row) {
				return ""
			}
			return strings.TrimSpace(row[n])
		}
		title, slug := get("title"), get("slug")
		price, pe := strconv.ParseInt(get("price_cents"), 10, 64)
		qty, qe := strconv.ParseInt(get("quantity"), 10, 64)
		if title == "" || slug == "" || get("option1_name") == "" || get("option1_value") == "" || pe != nil || qe != nil || price < 0 || qty < 0 {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "需提供标题、slug、第一变体轴及非负整数价格/库存"})
			continue
		}
		g := groups[slug]
		if g == nil {
			status := get("status")
			if status == "" {
				status = "draft"
			}
			g = &grouped{in: catalog.ProductInput{Title: title, Slug: slug, Status: status, Description: get("description")}}
			groups[slug] = g
			order = append(order, slug)
		} else if g.in.Title != title || g.in.Status != get("status") && get("status") != "" {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "同一 slug 的标题或状态必须一致"})
			continue
		}
		sels := []catalog.Selection{{Option: get("option1_name"), Value: get("option1_value")}}
		if n, v := get("option2_name"), get("option2_value"); n != "" || v != "" {
			if n == "" || v == "" {
				p.Errors = append(p.Errors, RowError{Row: line, Message: "第二变体轴名称和取值必须同时提供"})
				continue
			}
			sels = append(sels, catalog.Selection{Option: n, Value: v})
		}
		g.variants = append(g.variants, catalog.VariantInput{SKU: get("sku"), PriceCents: price, Quantity: qty, Selections: sels})
		p.VariantCount++
	}
	inputs := make([]catalog.ProductInput, 0, len(order))
	for _, slug := range order {
		g := groups[slug]
		axes := map[string][]string{}
		axisOrder := []string{}
		for _, v := range g.variants {
			for _, s := range v.Selections {
				known := false
				for _, x := range axes[s.Option] {
					if x == s.Value {
						known = true
					}
				}
				if _, ok := axes[s.Option]; !ok {
					axisOrder = append(axisOrder, s.Option)
				}
				if !known {
					axes[s.Option] = append(axes[s.Option], s.Value)
				}
			}
		}
		for _, name := range axisOrder {
			g.in.Options = append(g.in.Options, catalog.OptionInput{Name: name, Values: axes[name]})
		}
		g.in.Variants = g.variants
		inputs = append(inputs, g.in)
	}
	p.ProductCount = len(inputs)
	return p, inputs
}

// ValidationError 表示可直接展示给商家的导入校验错误。
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
