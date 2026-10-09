package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"cserver/internal/core"
)

type inventoryFixture struct {
	servers []core.DiscoveredServer
	err     error
}

func (r inventoryFixture) Discover(
	_ context.Context,
	_ string,
) ([]core.DiscoveredServer, error) {
	return r.servers, r.err
}

type batchFixture struct {
	calls    int
	metadata map[string]core.InfrastructureMetadata
	err      error
}

func (b *batchFixture) InspectAll(
	_ context.Context,
	_ []core.DiscoveredServer,
) (map[string]core.InfrastructureMetadata, error) {
	b.calls++
	return b.metadata, b.err
}

func TestDiscoveryServiceEnrichesOnceAndExposesOptionalJSON(t *testing.T) {
	root := t.TempDir()

	original := []core.DiscoveredServer{
		{
			ID:        "aim",
			Name:      "AIM",
			Directory: filepath.Join(root, "aim"),
			Status:    core.DiscoveryComplete,
			Evidence:  []string{"env", "compose"},
		},
		{
			ID:        "skills",
			Name:      "Skills",
			Directory: filepath.Join(root, "skills"),
			Status:    core.DiscoveryIncomplete,
			Evidence:  []string{"env"},
		},
	}

	batch := &batchFixture{
		metadata: map[string]core.InfrastructureMetadata{
			"aim": {
				OverlayFS: &core.OverlayFSMetadata{
					Source:       "systemd_unit",
					RuntimeState: core.RuntimeNotChecked,
				},
				Systemd: &core.SystemdMountMetadata{
					UnitName:     "aim.mount",
					UnitFile:     filepath.Join(root, "units", "aim.mount"),
					Where:        filepath.Join(root, "aim", "merged"),
					RuntimeState: core.RuntimeNotChecked,
				},
			},
		},
	}

	svc, err := NewServerDiscoveryServiceWithInfrastructure(
		root,
		inventoryFixture{servers: original},
		batch,
	)
	if err != nil {
		t.Fatal(err)
	}

	servers, err := svc.List(context.Background())
	if err != nil || batch.calls != 1 || len(servers) != 2 {
		t.Fatalf(
			"unexpected discovery: %#v (%v), calls %d",
			servers,
			err,
			batch.calls,
		)
	}

	if servers[0].Infrastructure == nil ||
		servers[0].Infrastructure.Systemd == nil ||
		servers[1].Infrastructure != nil {
		t.Fatalf("incorrect enrichment: %#v", servers)
	}

	// Stage 2C.3 exposes optional infrastructure metadata in public JSON.
	data, err := json.Marshal(servers)
	if err != nil {
		t.Fatal(err)
	}

	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if len(decoded) != 2 {
		t.Fatalf("expected two JSON servers, got %d", len(decoded))
	}

	// Existing public fields must remain available.
	for _, key := range []string{
		"id",
		"name",
		"directory",
		"status",
		"evidence",
	} {
		if _, ok := decoded[0][key]; !ok {
			t.Fatalf("existing public field %q missing", key)
		}
	}

	// Enriched instances must expose infrastructure metadata.
	rawInfrastructure, ok := decoded[0]["infrastructure"]
	if !ok {
		t.Fatalf(
			"enriched instance lacks infrastructure in JSON: %s",
			data,
		)
	}

	// Instances without infrastructure must omit the optional field.
	if _, ok := decoded[1]["infrastructure"]; ok {
		t.Fatalf(
			"instance without metadata unexpectedly exposes infrastructure: %s",
			data,
		)
	}

	var public core.InfrastructureMetadata
	if err := json.Unmarshal(rawInfrastructure, &public); err != nil {
		t.Fatal(err)
	}

	if public.OverlayFS == nil ||
		public.Systemd == nil ||
		public.OverlayFS.RuntimeState != core.RuntimeNotChecked ||
		public.Systemd.RuntimeState != core.RuntimeNotChecked ||
		public.Systemd.UnitName != "aim.mount" {
		t.Fatalf("unexpected public infrastructure metadata: %#v", public)
	}
}

func TestDiscoveryServicePreservesOriginalConstructor(t *testing.T) {
	svc, err := NewServerDiscoveryService(
		t.TempDir(),
		inventoryFixture{
			servers: []core.DiscoveredServer{
				{ID: "base"},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.List(context.Background())
	if err != nil ||
		len(result) != 1 ||
		result[0].Infrastructure != nil {
		t.Fatalf("unexpected legacy result: %#v %v", result, err)
	}

	// Without infrastructure configuration, the JSON must remain compatible.
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}

	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}

	if _, present := decoded[0]["infrastructure"]; present {
		t.Fatalf(
			"unconfigured infrastructure unexpectedly serialized: %s",
			data,
		)
	}
}

func TestDiscoveryServiceInfrastructureFailuresAndEmptyInventory(t *testing.T) {
	marker := errors.New("fixture infrastructure failure")

	batch := &batchFixture{err: marker}

	svc, err := NewServerDiscoveryServiceWithInfrastructure(
		t.TempDir(),
		inventoryFixture{
			servers: []core.DiscoveredServer{
				{ID: "aim"},
			},
		},
		batch,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = svc.List(context.Background())
	if !errors.Is(err, marker) || batch.calls != 1 {
		t.Fatalf("unexpected failure %v", err)
	}

	batch.calls = 0

	svc, err = NewServerDiscoveryServiceWithInfrastructure(
		t.TempDir(),
		inventoryFixture{},
		batch,
	)
	if err != nil {
		t.Fatal(err)
	}

	result, err := svc.List(context.Background())
	if err != nil || len(result) != 0 || batch.calls != 0 {
		t.Fatalf(
			"empty inventory should not scan: %v %v calls %d",
			result,
			err,
			batch.calls,
		)
	}

	batch.calls = 0

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = svc.List(ctx)
	if !errors.Is(err, context.Canceled) || batch.calls != 0 {
		t.Fatalf(
			"cancellation failure %v calls %d",
			err,
			batch.calls,
		)
	}
}
