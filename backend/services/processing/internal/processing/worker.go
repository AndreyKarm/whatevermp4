package processing

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"movie-platform/shared/serviceclient"
)

// Worker polls for queued jobs and runs them through the pipeline stages,
// reporting progress back to Catalog via a signed internal service call.
// Replace the sleeps with real ffmpeg/packaging calls and real download logic.
type Worker struct {
	repo         *Repository
	catalogURL   string
	client       *serviceclient.Client
	pollInterval time.Duration
}

func NewWorker(repo *Repository, internalSecret []byte) *Worker {
	return &Worker{
		repo:         repo,
		catalogURL:   os.Getenv("CATALOG_SERVICE_URL"),
		client:       serviceclient.New(internalSecret, "processing"),
		pollInterval: 3 * time.Second,
	}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			job, err := w.repo.NextQueuedJob(ctx)
			if err != nil {
				if !errors.Is(err, ErrNotFound) {
					log.Printf("failed to fetch next job: %v", err)
				}
				continue
			}
			w.process(ctx, job)
		}
	}
}

func (w *Worker) process(ctx context.Context, job *Job) {
	stages := []string{"downloading", "transcoding", "packaging", "done"}
	for _, stage := range stages {
		if err := w.repo.UpdateStage(ctx, job.ID, stage, 0, nil); err != nil {
			log.Printf("job %s: failed to update local stage: %v", job.ID, err)
			return
		}
		time.Sleep(1 * time.Second) // placeholder for real ffmpeg/packaging work
		w.notifyCatalog(job.VersionID, stage)
	}
}

func (w *Worker) notifyCatalog(versionID, status string) {
	if w.catalogURL == "" {
		return
	}

	url := w.catalogURL + "/catalog/versions/" + versionID + "/status"
	resp, err := w.client.Patch(url, map[string]string{"status": status})
	if err != nil {
		log.Printf("failed to notify catalog for version %s: %v", versionID, err)
		return
	}
	if err := serviceclient.CheckStatus(resp); err != nil {
		log.Printf("catalog rejected status update for version %s: %v", versionID, err)
	}
}
