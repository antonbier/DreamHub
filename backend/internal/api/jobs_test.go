package api_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/antonbier/DreamHub/backend/internal/store"
)

type jobsList struct {
	Jobs []struct {
		ID      int64  `json:"id"`
		Kind    string `json:"kind"`
		OwnerID int64  `json:"ownerId"`
		State   string `json:"state"`
	} `json:"jobs"`
	BlockedBy string `json:"blockedBy"`
}

func TestJobs_OwnersSeeAndCancelTheirOwn(t *testing.T) {
	e := newEnv(t)
	admin, viewer, viewerID := e.household(t)
	ctx := context.Background()
	var me whoami
	e.json(t, "GET", "/api/v1/whoami", "", withProfile(admin), 200, &me)

	// The queue isn't running in tests; queued jobs are enough to check
	// who sees and cancels what.
	mine, err := e.store.InsertJob(ctx, store.Job{Kind: "media.adscan", OwnerID: viewerID, State: store.JobQueued})
	if err != nil {
		t.Fatal(err)
	}
	theirs, err := e.store.InsertJob(ctx, store.Job{Kind: "media.adscan", OwnerID: me.ID, State: store.JobQueued})
	if err != nil {
		t.Fatal(err)
	}

	var list jobsList
	e.json(t, "GET", "/api/v1/jobs", "", withProfile(viewer), 200, &list)
	if len(list.Jobs) != 1 || list.Jobs[0].ID != mine.ID {
		t.Fatalf("a viewer sees only their own jobs, got %+v", list.Jobs)
	}
	e.json(t, "GET", "/api/v1/jobs", "", withProfile(admin), 200, &list)
	if len(list.Jobs) != 2 {
		t.Fatalf("an admin sees every job, got %+v", list.Jobs)
	}

	// Someone else's job looks like no job at all.
	for _, path := range []string{"/api/v1/jobs/" + itoa(theirs.ID), "/api/v1/jobs/999"} {
		resp, b := e.do(t, "GET", path, "", withProfile(viewer))
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s as viewer: %d %s", path, resp.StatusCode, b)
		}
	}
	resp, _ := e.do(t, "POST", "/api/v1/jobs/"+itoa(theirs.ID)+"/cancel", "", withProfile(viewer))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a viewer cancelled someone else's job: %d", resp.StatusCode)
	}

	var job struct {
		State string `json:"state"`
	}
	e.json(t, "POST", "/api/v1/jobs/"+itoa(mine.ID)+"/cancel", "", withProfile(viewer), 200, &job)
	if job.State != store.JobCanceled {
		t.Fatalf("cancelled job state %q", job.State)
	}
	resp, _ = e.do(t, "POST", "/api/v1/jobs/"+itoa(mine.ID)+"/cancel", "", withProfile(viewer))
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("cancelling twice: %d, want 409", resp.StatusCode)
	}
	// An admin may cancel anyone's.
	e.json(t, "POST", "/api/v1/jobs/"+itoa(theirs.ID)+"/cancel", "", withProfile(admin), 200, nil)
}

// A profile limited to some boxes hears nothing of the jobs on the others,
// whatever its role.
func TestJobs_NotThoseOfABoxTheProfileMayNotUse(t *testing.T) {
	e := newEnv(t)
	admin, viewer, viewerID := e.household(t)
	e.addBox(t, "bedroom")
	ctx := context.Background()
	here, err := e.store.InsertJob(ctx, store.Job{Kind: "media.adscan", OwnerID: viewerID, BoxID: "main", State: store.JobQueued})
	if err != nil {
		t.Fatal(err)
	}
	away, err := e.store.InsertJob(ctx, store.Job{Kind: "media.adscan", OwnerID: viewerID, BoxID: "bedroom", State: store.JobQueued})
	if err != nil {
		t.Fatal(err)
	}
	e.json(t, "PATCH", "/api/v1/profiles/"+itoa(viewerID), `{"boxes":["main"]}`, withProfile(admin), 200, nil)
	var list jobsList
	e.json(t, "GET", "/api/v1/jobs", "", withProfile(viewer), 200, &list)
	if len(list.Jobs) != 1 || list.Jobs[0].ID != here.ID {
		t.Fatalf("only the job on the allowed box is listed, got %+v", list.Jobs)
	}
	resp, _ := e.do(t, "GET", "/api/v1/jobs/"+itoa(away.ID), "", withProfile(viewer))
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("a job on a forbidden box looks like no job: %d", resp.StatusCode)
	}
}
