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
	"html"
	"io"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/kartwo/kartwo/internal/catalog"
	"github.com/kartwo/kartwo/internal/media"
	"github.com/kartwo/kartwo/internal/redirect"
)

const maxCSVBytes = 5 << 20

const (
	FormatGeneric = "generic"
	FormatShopify = "shopify"
)

var requiredColumns = []string{"title", "slug", "status", "description", "option1_name", "option1_value", "option2_name", "option2_value", "sku", "price_cents", "quantity"}

type RowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}
type Preview struct {
	PublicID     string     `json:"public_id"`
	Format       string     `json:"format"`
	Status       string     `json:"status"`
	TotalRows    int        `json:"total_rows"`
	ProductCount int        `json:"product_count"`
	VariantCount int        `json:"variant_count"`
	Errors       []RowError `json:"errors"`
}
type Service struct {
	db       *sql.DB
	catalog  *catalog.Service
	media    *media.Service
	redirect *redirect.Service
}

func New(db *sql.DB, catalogSvc *catalog.Service, mediaSvc *media.Service, redirectSvc *redirect.Service) *Service {
	return &Service{db: db, catalog: catalogSvc, media: mediaSvc, redirect: redirectSvc}
}

// PreviewCSV 解析并持久化一次干跑。相同字节内容返回原任务，不会产生第二份任务。
func (s *Service) PreviewCSV(ctx context.Context, src io.Reader) (Preview, error) {
	return s.preview(ctx, src, FormatGeneric)
}

// PreviewShopifyCSV 以 Shopify 商品 CSV 协议干跑预览；远程图片在执行阶段才下载。
func (s *Service) PreviewShopifyCSV(ctx context.Context, src io.Reader) (Preview, error) {
	return s.preview(ctx, src, FormatShopify)
}

func (s *Service) preview(ctx context.Context, src io.Reader, format string) (Preview, error) {
	b, err := io.ReadAll(io.LimitReader(src, maxCSVBytes+1))
	if err != nil {
		return Preview{}, fmt.Errorf("import: 读取 CSV: %w", err)
	}
	if len(b) > maxCSVBytes {
		return Preview{}, &ValidationError{Message: "CSV 不能超过 5MB"}
	}
	h := sha256.Sum256(append([]byte(formatHashKey(format)+"\x00"), b...))
	hash := hex.EncodeToString(h[:])
	if old, err := s.getByHash(ctx, hash); err == nil {
		return old, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return Preview{}, err
	}
	parsed := parseForFormat(format, b)
	p := parsed.preview
	p.PublicID = uuid.Must(uuid.NewV7()).String()
	p.Format = format
	if len(p.Errors) == 0 && len(parsed.inputs) > 0 {
		p.Status = "previewed"
	} else {
		p.Status = "rejected"
	}
	errs, _ := json.Marshal(p.Errors)
	_, err = s.db.ExecContext(ctx, `INSERT INTO import_job (public_id, source_sha256, source_csv, source_format, status, total_rows, product_count, variant_count, errors_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, p.PublicID, hash, string(b), format, p.Status, p.TotalRows, p.ProductCount, p.VariantCount, string(errs))
	if err != nil {
		return Preview{}, fmt.Errorf("import: 保存预览: %w", err)
	}
	return p, nil
}

// formatHashKey 随解析语义变化升级，避免历史被拒绝预览阻止修复后的同一文件重试。
func formatHashKey(format string) string {
	switch format {
	case FormatShopify:
		return "shopify-v2-images"
	default:
		return format
	}
}

// Execute 将一个无错误预览一次性写入；成功任务重复执行只返回原结果。
func (s *Service) Execute(ctx context.Context, publicID string) (Preview, error) {
	// 图片先在事务外下载、校验并处理，避免在数据库事务中等待网络。
	p, err := s.Get(ctx, publicID)
	if err != nil {
		return Preview{}, err
	}
	if p.Status == "succeeded" {
		return p, nil
	}
	if p.Status != "previewed" {
		return Preview{}, &ValidationError{Message: "该导入任务存在行错误，不能执行"}
	}
	_, source, err := getByID(ctx, s.db, publicID)
	if err != nil {
		return Preview{}, err
	}
	parsed := parseForFormat(p.Format, []byte(source))
	if len(parsed.preview.Errors) != 0 || len(parsed.inputs) == 0 {
		return Preview{}, &ValidationError{Message: "导入源已无效，请重新预览"}
	}
	prepared, err := prepareRemoteImages(ctx, parsed.images)
	if err != nil {
		return Preview{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Preview{}, fmt.Errorf("import: 开启事务: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	p, _, err = getByID(ctx, tx, publicID)
	if err != nil {
		return Preview{}, err
	}
	if p.Status == "succeeded" {
		return p, nil
	}
	if p.Status != "previewed" {
		return Preview{}, &ValidationError{Message: "该导入任务存在行错误，不能执行"}
	}
	ids, err := s.catalog.CreateProductsTx(ctx, tx, parsed.inputs)
	if err != nil {
		return Preview{}, err
	}
	if p.Format == FormatShopify {
		if err := s.storeShopifyRedirects(ctx, tx, ids, parsed.inputs); err != nil {
			return Preview{}, err
		}
	}
	if err := s.storeImportedImages(ctx, tx, ids, parsed.inputs, prepared); err != nil {
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

func (s *Service) storeShopifyRedirects(ctx context.Context, tx *sql.Tx, ids []string, inputs []catalog.ProductInput) error {
	if s.redirect == nil {
		return fmt.Errorf("import: Shopify 重定向服务未配置")
	}
	for i, publicID := range ids {
		productID, err := s.catalog.ProductIDByPublicIDTx(ctx, tx, publicID)
		if err != nil {
			return err
		}
		if err := s.redirect.StoreShopifyTx(ctx, tx, inputs[i].Slug, productID); err != nil {
			return err
		}
	}
	return nil
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

func (s *Service) storeImportedImages(ctx context.Context, tx *sql.Tx, ids []string, inputs []catalog.ProductInput, prepared []preparedImage) error {
	if len(prepared) == 0 {
		return nil
	}
	if s.media == nil {
		return fmt.Errorf("import: 图片服务未配置")
	}
	productIDs := make(map[string]int64, len(ids))
	for i, publicID := range ids {
		productID, err := s.catalog.ProductIDByPublicIDTx(ctx, tx, publicID)
		if err != nil {
			return err
		}
		productIDs[inputs[i].Slug] = productID
	}
	for _, img := range prepared {
		productID, ok := productIDs[img.Slug]
		if !ok {
			return fmt.Errorf("import: 图片引用的商品不存在")
		}
		if err := s.media.StoreProcessedTx(ctx, tx, productID, img.Processed); err != nil {
			return fmt.Errorf("import: 第 %d 行图片写入失败: %w", img.Row, err)
		}
	}
	return nil
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getByID(ctx context.Context, q queryer, id string) (Preview, string, error) {
	var p Preview
	var raw string
	var source string
	err := q.QueryRowContext(ctx, `SELECT public_id,source_format,status,total_rows,product_count,variant_count,errors_json,source_csv FROM import_job WHERE public_id=?`, id).Scan(&p.PublicID, &p.Format, &p.Status, &p.TotalRows, &p.ProductCount, &p.VariantCount, &raw, &source)
	if err != nil {
		return Preview{}, "", err
	}
	if err := json.Unmarshal([]byte(raw), &p.Errors); err != nil {
		return Preview{}, "", fmt.Errorf("import: 读错误报告: %w", err)
	}
	return p, source, nil
}

type parsedImport struct {
	preview Preview
	inputs  []catalog.ProductInput
	images  []imageReference
}

func parseForFormat(format string, b []byte) parsedImport {
	switch format {
	case FormatGeneric:
		p, inputs := parseCSV(b)
		return parsedImport{preview: p, inputs: inputs}
	case FormatShopify:
		return parseShopifyCSV(b)
	default:
		return parsedImport{preview: Preview{Errors: []RowError{{Row: 1, Message: "不支持的导入来源"}}}}
	}
}

type grouped struct {
	in       catalog.ProductInput
	variants []catalog.VariantInput
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
	return buildInputs(p, groups, order)
}

func buildInputs(p Preview, groups map[string]*grouped, order []string) (Preview, []catalog.ProductInput) {
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

var shopifyColumns = []string{"Handle", "Title", "Body (HTML)", "Option1 Name", "Option1 Value", "Option2 Name", "Option2 Value", "Option3 Name", "Option3 Value", "Variant SKU", "Variant Inventory Qty", "Variant Price"}

// parseShopifyCSV 将 Shopify 的商品/变体行转为内核通用商品输入，并收集图片引用。
func parseShopifyCSV(b []byte) parsedImport {
	r := csv.NewReader(strings.NewReader(string(b)))
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	head, err := r.Read()
	if err != nil {
		return parsedImport{preview: Preview{Errors: []RowError{{Row: 1, Message: "CSV 缺少表头"}}}}
	}
	idx := map[string]int{}
	for i, h := range head {
		idx[strings.TrimSpace(h)] = i
	}
	for _, name := range shopifyColumns {
		if _, ok := idx[name]; !ok {
			return parsedImport{preview: Preview{Errors: []RowError{{Row: 1, Message: "不是完整的 Shopify 商品 CSV，缺少列 " + name}}}}
		}
	}
	groups := map[string]*grouped{}
	order := []string{}
	images := []imageReference{}
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
		handle := get("Handle")
		if handle == "" {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "需提供 Handle"})
			continue
		}
		if get("Option3 Name") != "" || get("Option3 Value") != "" {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "暂不支持 Shopify 第三变体轴"})
			continue
		}
		if imageIndex, ok := idx["Image Src"]; ok && imageIndex < len(row) && strings.TrimSpace(row[imageIndex]) != "" {
			imageURL := strings.TrimSpace(row[imageIndex])
			if err := validateRemoteImageURL(imageURL); err != nil {
				p.Errors = append(p.Errors, RowError{Row: line, Message: err.Error()})
				continue
			}
			images = append(images, imageReference{Slug: handle, Row: line, URL: imageURL})
		}
		// Shopify 会将额外图片单独输出为只有 Handle 和 Image Src 的行。
		if get("Variant Price") == "" && get("Variant Inventory Qty") == "" && get("Option1 Name") == "" && get("Option1 Value") == "" {
			if groups[handle] == nil {
				p.Errors = append(p.Errors, RowError{Row: line, Message: "图片行必须跟随商品或变体行"})
			}
			continue
		}
		g := groups[handle]
		if g == nil {
			title := get("Title")
			if title == "" {
				p.Errors = append(p.Errors, RowError{Row: line, Message: "首行需提供 Title"})
				continue
			}
			status := "draft"
			if v, ok := idx["Status"]; ok && v < len(row) && strings.TrimSpace(row[v]) != "" {
				status = strings.TrimSpace(row[v])
			} else if v, ok := idx["Published"]; ok && v < len(row) && strings.EqualFold(strings.TrimSpace(row[v]), "true") {
				status = "active"
			}
			g = &grouped{in: catalog.ProductInput{Title: title, Slug: handle, Status: status, Description: plainText(get("Body (HTML)"))}}
			groups[handle] = g
			order = append(order, handle)
		}
		price, pe := shopifyPriceCents(get("Variant Price"))
		qtyText := get("Variant Inventory Qty")
		qty := int64(0)
		var qe error
		if qtyText != "" {
			qty, qe = strconv.ParseInt(qtyText, 10, 64)
		}
		if pe != nil || qe != nil || price < 0 || qty < 0 {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "需提供非负的 Variant Price 与 Variant Inventory Qty"})
			continue
		}
		o1n, o1v := get("Option1 Name"), get("Option1 Value")
		sels := []catalog.Selection{}
		if o1n == "" && o1v == "" {
			sels = append(sels, catalog.Selection{Option: "Title", Value: "Default Title"})
		} else if o1n == "" || o1v == "" {
			p.Errors = append(p.Errors, RowError{Row: line, Message: "Option1 Name 与 Option1 Value 必须同时提供"})
			continue
		} else {
			sels = append(sels, catalog.Selection{Option: o1n, Value: o1v})
		}
		o2n, o2v := get("Option2 Name"), get("Option2 Value")
		if o2n != "" || o2v != "" {
			if o2n == "" || o2v == "" {
				p.Errors = append(p.Errors, RowError{Row: line, Message: "Option2 Name 与 Option2 Value 必须同时提供"})
				continue
			}
			sels = append(sels, catalog.Selection{Option: o2n, Value: o2v})
		}
		g.variants = append(g.variants, catalog.VariantInput{SKU: get("Variant SKU"), PriceCents: price, Quantity: qty, Selections: sels})
		p.VariantCount++
	}
	p, inputs := buildInputs(p, groups, order)
	return parsedImport{preview: p, inputs: inputs, images: images}
}

func shopifyPriceCents(v string) (int64, error) {
	parts := strings.Split(strings.TrimSpace(v), ".")
	if len(parts) == 0 || len(parts) > 2 || parts[0] == "" {
		return 0, errors.New("invalid price")
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	if len(fraction) > 2 {
		return 0, errors.New("too many decimals")
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	frac := int64(0)
	if fraction != "" {
		frac, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	return whole*100 + frac, nil
}

func plainText(v string) string {
	v = html.UnescapeString(v)
	for {
		start := strings.Index(v, "<")
		if start < 0 {
			break
		}
		end := strings.Index(v[start:], ">")
		if end < 0 {
			break
		}
		v = v[:start] + v[start+end+1:]
	}
	return strings.TrimSpace(v)
}

// ValidationError 表示可直接展示给商家的导入校验错误。
type ValidationError struct{ Message string }

func (e *ValidationError) Error() string { return e.Message }
