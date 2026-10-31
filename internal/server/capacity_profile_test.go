// 受限资源程序验收 / Constrained Binary Capacity Profile
// 功能：显式启用时在1CPU/1GiB Linux容器运行实际二进制，核对导入/万单查询/媒体与资源峰值
// 作者：仗键天涯(daxing)
// 邮箱：3442535897@qq.com
// 时间：2026-10-26 19:03:59
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"debug/buildinfo"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
)

type capacityClient struct {
	base   string
	client *http.Client
}

func (c capacityClient) call(method, path string, payload []byte, contentType string, want int) ([]byte, error) {
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Origin", c.base)
	parsed, err := url.Parse(c.base)
	if err != nil {
		return nil, err
	}
	for _, cookie := range c.client.Jar.Cookies(parsed) {
		if cookie.Name == "kartwo_csrf" {
			req.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if len(body) >= 8<<20 {
		return nil, fmt.Errorf("capacity response exceeds bounded reader")
	}
	if resp.StatusCode != want {
		return nil, fmt.Errorf("%s %s status %d want %d", method, path, resp.StatusCode, want)
	}
	if strings.HasPrefix(path, "/admin/api/") && (!strings.Contains(resp.Header.Get("Cache-Control"), "no-store") || resp.Header.Get("Referrer-Policy") != "no-referrer") {
		return nil, fmt.Errorf("private response headers absent")
	}
	return body, nil
}

func (c capacityClient) json(t *testing.T, method, path string, payload any, want int) []byte {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.call(method, path, body, "application/json", want)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func capacityMultipart(t *testing.T, name string, content []byte, extra map[string]string) ([]byte, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = file.Write(content); err != nil {
		t.Fatal(err)
	}
	for key, value := range extra {
		if err = writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes(), writer.FormDataContentType()
}

func TestConstrainedBinaryCapacityProfile(t *testing.T) {
	binary := os.Getenv("KARTWO_CAPACITY_BINARY")
	if binary == "" {
		t.Skip("opt-in: actual Linux binary and 1CPU/1GiB cgroup required")
	}
	output := os.Getenv("KARTWO_CAPACITY_OUTPUT")
	if runtime.GOOS != "linux" || output == "" {
		t.Fatal("Linux and private output directory required")
	}
	binaryInfo, err := buildinfo.ReadFile(binary)
	if err != nil {
		t.Fatal("read actual binary build info", err)
	}
	buildSettings := map[string]string{}
	for _, setting := range binaryInfo.Settings {
		switch setting.Key {
		case "GOOS", "GOARCH", "CGO_ENABLED", "-tags":
			buildSettings[setting.Key] = setting.Value
		}
	}
	if buildSettings["GOOS"] != "linux" || buildSettings["GOARCH"] != runtime.GOARCH || buildSettings["CGO_ENABLED"] != "0" || buildSettings["-tags"] != "nodynamic" {
		t.Fatal("capacity binary must use the official Linux static build settings")
	}
	cpu, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		t.Fatal(err)
	}
	quota := strings.Fields(string(cpu))
	if len(quota) != 2 || quota[0] != quota[1] {
		t.Fatal("cgroup must cap exactly one CPU")
	}
	if value, parseErr := strconv.ParseInt(quota[0], 10, 64); parseErr != nil || value <= 0 {
		t.Fatal("invalid CPU quota")
	}
	memory, err := os.ReadFile("/sys/fs/cgroup/memory.max")
	if err != nil || strings.TrimSpace(string(memory)) != "1073741824" {
		t.Fatal("cgroup must cap exactly 1GiB")
	}
	if err = os.MkdirAll(output, 0700); err != nil {
		t.Fatal(err)
	}
	if filepath.Clean(os.TempDir()) != filepath.Clean(output) {
		t.Fatal("TMPDIR must point at the dedicated output bind mount")
	}
	mountInfo, mountErr := os.ReadFile("/proc/self/mountinfo")
	if mountErr != nil {
		t.Fatal(mountErr)
	}
	filesystem := ""
	for _, line := range strings.Split(string(mountInfo), "\n") {
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		fields := strings.Fields(parts[0])
		if len(fields) > 4 && fields[4] == output {
			filesystem = strings.Fields(parts[1])[0]
		}
	}
	if filesystem == "" || filesystem == "tmpfs" {
		t.Fatal("capacity database must use a dedicated file-backed mount")
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(output, "server.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = log.Close() }()
	child := exec.Command(binary, "serve")
	child.Env = []string{"KARTWO_ENV=dev", "KARTWO_DATA_DIR=" + dataDir, "KARTWO_ADDR=" + addr, "KARTWO_BASE_URL=http://" + addr, "KARTWO_BACKUP_WEBDAV_ENABLED=false"}
	child.Stdout, child.Stderr = log, log
	started := time.Now()
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- child.Wait() }()
	stopped := false
	t.Cleanup(func() {
		if stopped {
			return
		}
		_ = child.Process.Signal(syscall.SIGINT)
		select {
		case stopErr := <-done:
			if stopErr != nil {
				t.Errorf("binary exit: %v", stopErr)
			}
		case <-time.After(10 * time.Second):
			_ = child.Process.Kill()
			waitErr := <-done
			t.Errorf("binary did not shut down gracefully (wait: %v)", waitErr)
		}
	})
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	c := capacityClient{base: "http://" + addr, client: &http.Client{Jar: jar, Timeout: 90 * time.Second}}
	deadline := time.Now().Add(40 * time.Second)
	for {
		reqCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		req, reqErr := http.NewRequestWithContext(reqCtx, http.MethodGet, c.base+"/health", nil)
		if reqErr != nil {
			cancel()
			t.Fatal(reqErr)
		}
		response, healthErr := c.client.Do(req)
		if healthErr == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				cancel()
				break
			}
		}
		cancel()
		select {
		case stopErr := <-done:
			stopped = true
			t.Fatalf("binary stopped before health: %v", stopErr)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("health startup timeout")
		}
		time.Sleep(50 * time.Millisecond)
	}
	version, versionErr := exec.Command(binary, "version").Output()
	if versionErr != nil || len(bytes.TrimSpace(version)) == 0 {
		t.Fatal("binary version unavailable", versionErr)
	}
	binaryFile, binaryErr := os.Open(binary)
	if binaryErr != nil {
		t.Fatal(binaryErr)
	}
	digest := sha256.New()
	if _, binaryErr = io.Copy(digest, binaryFile); binaryErr != nil {
		_ = binaryFile.Close()
		t.Fatal(binaryErr)
	}
	if binaryErr = binaryFile.Close(); binaryErr != nil {
		t.Fatal(binaryErr)
	}
	report := map[string]any{"binary_sha256": fmt.Sprintf("%x", digest.Sum(nil)), "database_filesystem": filesystem, "source_commit": os.Getenv("KARTWO_CAPACITY_COMMIT"), "binary_version": strings.TrimSpace(string(version)), "platform": runtime.GOOS + "/" + runtime.GOARCH, "cpu_max": strings.TrimSpace(string(cpu)), "memory_max_bytes": int64(1 << 30), "startup_ms": time.Since(started).Milliseconds(), "source": "actual child binary; isolated synthetic data; no external network"}
	report["binary_build_settings"] = buildSettings
	password := rand.Text()
	c.json(t, "POST", "/admin/api/setup", map[string]string{"username": "capacity-owner", "password": password}, http.StatusCreated)
	c.json(t, "POST", "/admin/api/login", map[string]string{"username": "capacity-owner", "password": password}, http.StatusOK)
	c.json(t, "PUT", "/admin/api/settings/market", map[string]string{"code": "US"}, http.StatusOK)

	var csv strings.Builder
	csv.WriteString("title,slug,status,description,option1_name,option1_value,option2_name,option2_value,sku,price_cents,quantity\n")
	for index := 0; index < 1000; index++ {
		for _, size := range []string{"S", "M"} {
			fmt.Fprintf(&csv, "Capacity Running Tee %d,capacity-running-%d,active,Synthetic capacity fixture,Size,%s,,,CAPACITY-%d-%s,1999,100\n", index, index, size, index, size)
		}
	}
	data, contentType := capacityMultipart(t, "capacity.csv", []byte(csv.String()), map[string]string{"format": "generic"})
	before := time.Now()
	body, err := c.call("POST", "/admin/api/imports/csv/preview", data, contentType, http.StatusOK)
	if err != nil {
		t.Fatal(err)
	}
	report["csv_preview_ms"] = time.Since(before).Milliseconds()
	var preview struct {
		ID       string `json:"public_id"`
		Products int    `json:"product_count"`
		Variants int    `json:"variant_count"`
		Status   string `json:"status"`
	}
	if err = json.Unmarshal(body, &preview); err != nil || preview.ID == "" || preview.Products != 1000 || preview.Variants != 2000 || preview.Status != "previewed" {
		t.Fatal("invalid capacity import preview", err)
	}
	before = time.Now()
	body = c.json(t, "POST", "/admin/api/imports/"+preview.ID+"/execute", nil, http.StatusOK)
	if !bytes.Contains(body, []byte(`"succeeded"`)) {
		t.Fatal("capacity import did not finish")
	}
	report["csv_execute_ms"] = time.Since(before).Milliseconds()

	db, err := sql.Open("sqlite", "file:"+filepath.Join(dataDir, "shop.db")+"?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	firstOrder := seedCapacityOrders(t, db)
	report["dataset"] = map[string]int{"products": 1000, "variants": 2000, "orders": 10000, "receipts": 8000, "shipments": 4000}

	var productID string
	if err = db.QueryRow(`SELECT public_id FROM product WHERE slug='capacity-running-0'`).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	pixels := image.NewNRGBA(image.Rect(0, 0, 2400, 1600))
	noise := uint32(1234567)
	for offset := 0; offset < len(pixels.Pix); offset += 4 {
		noise ^= noise << 13
		noise ^= noise >> 17
		noise ^= noise << 5
		pixels.Pix[offset], pixels.Pix[offset+1], pixels.Pix[offset+2], pixels.Pix[offset+3] = byte(noise), byte(noise>>8), byte(noise>>16), 255
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, pixels, &jpeg.Options{Quality: 92}); err != nil {
		t.Fatal(err)
	}
	if encoded.Len() < 2<<20 || encoded.Len() > 5<<20 {
		t.Fatal("capacity image outside representative 2–5MiB range")
	}
	report["image_input_bytes"] = encoded.Len()
	pixels = nil
	data, contentType = capacityMultipart(t, "capacity.jpg", encoded.Bytes(), nil)
	runtime.GC()
	before = time.Now()
	body, err = c.call("POST", "/admin/api/products/"+productID+"/media", data, contentType, http.StatusCreated)
	if err != nil {
		t.Fatal(err)
	}
	report["image_upload_ms"] = time.Since(before).Milliseconds()
	var media struct {
		Width       int `json:"width"`
		Height      int `json:"height"`
		Derivatives []struct {
			URL string `json:"url"`
		} `json:"derivatives"`
	}
	if err = json.Unmarshal(body, &media); err != nil || media.Width != 2400 || media.Height != 1600 || len(media.Derivatives) == 0 {
		t.Fatal("invalid capacity media response", err)
	}
	derived, err := c.call("GET", media.Derivatives[0].URL, nil, "", http.StatusOK)
	if err != nil || !bytes.HasPrefix(derived, []byte("RIFF")) {
		t.Fatal("capacity WebP unavailable", err)
	}
	report["image_derivatives"] = len(media.Derivatives)

	queries := []struct {
		name    string
		payload map[string]any
		total   int
	}{
		{"all", map[string]any{}, 10000},
		{"paid", map[string]any{"status": "paid"}, 2000},
		{"awaiting_payment", map[string]any{"view": "awaiting_payment"}, 2000},
		{"ready_to_fulfill", map[string]any{"view": "ready_to_fulfill"}, 2000},
		{"needs_review", map[string]any{"view": "needs_review"}, 2000},
		{"shipped", map[string]any{"view": "shipped"}, 4000},
		{"refunds_pending", map[string]any{"view": "refunds_pending"}, 0},
		{"one_id", map[string]any{"term": firstOrder}, 1},
		{"empty", map[string]any{"term": "no-match-capacity"}, 0},
	}
	latency := map[string]any{}
	for _, query := range queries {
		query.payload["page_size"] = 25
		samples := []int64{}
		for attempt := 0; attempt < 8; attempt++ {
			before = time.Now()
			result := c.json(t, "POST", "/admin/api/orders/search", query.payload, http.StatusOK)
			samples = append(samples, time.Since(before).Microseconds())
			if err = checkCapacityOrderPage(result, query.total); err != nil {
				t.Fatal(query.name, err)
			}
		}
		latency[query.name] = capacityQuantiles(samples)
	}
	samples := make(chan int64, 20)
	failures := make(chan error, 20)
	var workers sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		workers.Go(func() {
			for attempt := 0; attempt < 5; attempt++ {
				began := time.Now()
				result, callErr := c.call("POST", "/admin/api/orders/search", []byte(`{"page_size":25}`), "application/json", http.StatusOK)
				samples <- time.Since(began).Microseconds()
				if callErr == nil {
					callErr = checkCapacityOrderPage(result, 10000)
				}
				if callErr != nil {
					failures <- callErr
				}
			}
		})
	}
	workers.Wait()
	close(samples)
	close(failures)
	for failure := range failures {
		t.Error(failure)
	}
	if t.Failed() {
		t.FailNow()
	}
	parallel := []int64{}
	for sample := range samples {
		parallel = append(parallel, sample)
	}
	latency["all_concurrency_4"] = capacityQuantiles(parallel)
	report["order_search_latency_us"] = latency
	for _, path := range []string{"/health", "/admin/", "/collections/all", "/p/capacity-running-0"} {
		if _, err = c.call("GET", path, nil, "", http.StatusOK); err != nil {
			t.Fatal(err)
		}
	}
	var products, variants, orders, receipts, shipments, emails, refunds int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM product),(SELECT count(*) FROM variant),(SELECT count(*) FROM "order"),(SELECT count(*) FROM payment_receipt),(SELECT count(*) FROM shipment),(SELECT count(*) FROM email_outbox),(SELECT count(*) FROM refund_request)`).Scan(&products, &variants, &orders, &receipts, &shipments, &emails, &refunds); err != nil || products != 1000 || variants != 2000 || orders != 10000 || receipts != 8000 || shipments != 4000 || emails != 0 || refunds != 0 {
		t.Fatal("capacity facts changed", err)
	}
	report["outbound_email_and_refund_requests"] = 0
	peak, err := os.ReadFile("/sys/fs/cgroup/memory.peak")
	if err != nil {
		t.Fatal(err)
	}
	peakBytes, err := strconv.ParseInt(strings.TrimSpace(string(peak)), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	report["cgroup_memory_peak_bytes"] = peakBytes
	events, err := os.ReadFile("/sys/fs/cgroup/memory.events")
	if err != nil {
		t.Fatal(err)
	}
	report["memory_events"] = string(events)
	oomFields := 0
	for _, line := range strings.Split(string(events), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && (fields[0] == "oom" || fields[0] == "oom_kill") {
			oomFields++
			if fields[1] != "0" {
				t.Fatal("profile encountered OOM")
			}
		}
	}
	if oomFields != 2 {
		t.Fatal("missing cgroup OOM counters")
	}
	_ = child.Process.Signal(syscall.SIGINT)
	select {
	case err = <-done:
		stopped = true
		if err != nil {
			t.Fatal("binary failed to shut down", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("graceful shutdown timeout")
	}
	report["elapsed_ms"] = time.Since(started).Milliseconds()
	report["binary_graceful_exit"] = true
	result, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(output, "report.json"), result, 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(string(result))
}

func seedCapacityOrders(t *testing.T, db *sql.DB) string {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	customerID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	result, err := tx.Exec(`INSERT INTO customer(public_id,email) VALUES(?,'capacity@example.invalid')`, customerID.String())
	if err != nil {
		t.Fatal(err)
	}
	customer, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	orderStmt, err := tx.Prepare(`INSERT INTO "order"(public_id,customer_id,status,email,ship_name,ship_address,currency,subtotal_cents,total_cents,payment_provider) VALUES(?,?,?,'capacity@example.invalid','Synthetic','Synthetic address','USD',1999,1999,'stripe')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = orderStmt.Close() }()
	receiptStmt, err := tx.Prepare(`INSERT INTO payment_receipt(order_id,provider,payment_ref,amount_cents,refunded_cents,currency,review_reason) VALUES(?,'stripe',?,1999,?,'USD',?)`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = receiptStmt.Close() }()
	shipStmt, err := tx.Prepare(`INSERT INTO shipment(order_id,carrier,tracking_number) VALUES(?,'Capacity','SYNTHETIC')`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shipStmt.Close() }()
	first := ""
	states := []string{"pending", "paid", "fulfilled", "refunded", "payment_review"}
	for index := 0; index < 10000; index++ {
		id, idErr := uuid.NewV7()
		if idErr != nil {
			t.Fatal(idErr)
		}
		if index == 0 {
			first = id.String()
		}
		state := states[index%len(states)]
		row, insertErr := orderStmt.Exec(id.String(), customer, state)
		if insertErr != nil {
			t.Fatal(insertErr)
		}
		internalID, idErr := row.LastInsertId()
		if idErr != nil {
			t.Fatal(idErr)
		}
		if state != "pending" {
			refunded, reason := 0, ""
			if state == "refunded" {
				refunded = 1999
			}
			if state == "payment_review" {
				reason = "duplicate_payment"
			}
			if _, insertErr = receiptStmt.Exec(internalID, id.String(), refunded, reason); insertErr != nil {
				t.Fatal(insertErr)
			}
		}
		if state == "fulfilled" || state == "refunded" {
			if _, insertErr = shipStmt.Exec(internalID); insertErr != nil {
				t.Fatal(insertErr)
			}
		}
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return first
}

func checkCapacityOrderPage(body []byte, total int) error {
	var page struct {
		Total  int `json:"total"`
		Orders []struct {
			ID string `json:"public_id"`
		} `json:"orders"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return err
	}
	expected := min(total, 25)
	if page.Total != total || len(page.Orders) != expected {
		return fmt.Errorf("page facts total%d rows%d want%d/%d", page.Total, len(page.Orders), total, expected)
	}
	seen := map[string]bool{}
	for _, row := range page.Orders {
		if row.ID == "" || seen[row.ID] {
			return fmt.Errorf("invalid or duplicate order row")
		}
		seen[row.ID] = true
	}
	return nil
}

func capacityQuantiles(samples []int64) map[string]int64 {
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return map[string]int64{"samples": int64(len(samples)), "median": (samples[(len(samples)-1)/2] + samples[len(samples)/2]) / 2, "p95": samples[(len(samples)*95+99)/100-1], "max": samples[len(samples)-1]}
}
