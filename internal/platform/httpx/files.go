package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"go_external_api_document_flow/internal/client"
	"go_external_api_document_flow/internal/dto"
	"go_external_api_document_flow/internal/pdf"
)

const (
	// AttachmentConcurrency is how many attachments are fetched from don_snab at once.
	AttachmentConcurrency = 6

	// MaxAttachmentsTotalBytes is the total per-PDF attachment budget.
	MaxAttachmentsTotalBytes = 192 << 20
)

var ErrAttachmentsTooLarge = errors.New("суммарный размер вложений превышает лимит")

func FetchAttachments(ctx context.Context, api *client.DonSnab, raw any, pathFor func(fileID int64) string) []pdf.Attachment {
	list, _ := raw.([]any)

	type job struct {
		index int
		id    int64
	}

	attachments := make([]pdf.Attachment, 0, len(list))
	jobs := make([]job, 0, len(list))

	for _, item := range list {
		file, ok := item.(map[string]any)
		if !ok {
			continue
		}

		jobs = append(jobs, job{index: len(attachments), id: Int64Of(file["id"])})
		attachments = append(attachments, pdf.Attachment{
			Name:      StrOf(file["originalName"]),
			SizeLabel: dto.SizeLabel(file["fileSize"]),
		})
	}

	semaphore := make(chan struct{}, AttachmentConcurrency)
	var wg sync.WaitGroup
	var totalBytes atomic.Int64

	for _, j := range jobs {
		wg.Add(1)
		go func(j job) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			content, err := api.Fetch(ctx, pathFor(j.id), nil)
			if err == nil && totalBytes.Add(int64(len(content))) > MaxAttachmentsTotalBytes {
				content, err = nil, ErrAttachmentsTooLarge
			}

			attachments[j.index].Content = content
			attachments[j.index].Err = err
		}(j)
	}

	wg.Wait()

	return attachments
}

func NoStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
}

func WritePDF(w http.ResponseWriter, filename string, content []byte) {
	NoStore(w)
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, SafeFilename(filename)))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.Write(content)
}

func SafeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, name)
}

func StrOf(v any) string {
	s, _ := v.(string)
	return s
}

func Int64Of(v any) int64 {
	if n, ok := v.(float64); ok {
		return int64(n)
	}
	return 0
}
