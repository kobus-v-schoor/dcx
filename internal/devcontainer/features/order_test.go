package features

import (
	"strings"
	"testing"
)

func TestOrderNoDeps(t *testing.T) {
	features := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a"}},
		{Ref: FeatureRef{ID: "b"}, Meta: FeatureMeta{ID: "b"}},
		{Ref: FeatureRef{ID: "c"}, Meta: FeatureMeta{ID: "c"}},
	}
	ordered, err := Ordered(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 3 {
		t.Fatalf("expected 3 features, got %d", len(ordered))
	}
	// With no deps, order should be deterministic (sorted by ID).
	ids := []string{ordered[0].Meta.ID, ordered[1].Meta.ID, ordered[2].Meta.ID}
	want := []string{"a", "b", "c"}
	for i := range want {
		if ids[i] != want[i] {
			t.Errorf("ordered[%d] = %q, want %q", i, ids[i], want[i])
		}
	}
}

func TestOrderInstallsAfter(t *testing.T) {
	features := []ResolvedFeature{
		{Ref: FeatureRef{ID: "b"}, Meta: FeatureMeta{ID: "b", InstallsAfter: []string{"a"}}},
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a"}},
	}
	ordered, err := Ordered(features, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ordered) != 2 {
		t.Fatalf("expected 2 features, got %d", len(ordered))
	}
	if ordered[0].Meta.ID != "a" {
		t.Errorf("expected a first, got %s", ordered[0].Meta.ID)
	}
	if ordered[1].Meta.ID != "b" {
		t.Errorf("expected b second, got %s", ordered[1].Meta.ID)
	}
}

func TestOrderCircular(t *testing.T) {
	features := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a", InstallsAfter: []string{"b"}}},
		{Ref: FeatureRef{ID: "b"}, Meta: FeatureMeta{ID: "b", InstallsAfter: []string{"a"}}},
	}
	_, err := Ordered(features, nil)
	if err == nil {
		t.Fatal("expected error for circular installsAfter")
	}
	if !strings.Contains(err.Error(), "circular") {
		t.Errorf("expected circular error, got: %v", err)
	}
}

func TestOrderDependsOnMissing(t *testing.T) {
	features := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a", DependsOn: map[string]interface{}{"missing": nil}}},
	}
	_, err := Ordered(features, nil)
	if err == nil {
		t.Fatal("expected error for missing dependsOn")
	}
	if !strings.Contains(err.Error(), "auto-resolution not yet supported") {
		t.Errorf("expected unsupported error, got: %v", err)
	}
}

func TestOrderOverrideUnsupported(t *testing.T) {
	features := []ResolvedFeature{
		{Ref: FeatureRef{ID: "a"}, Meta: FeatureMeta{ID: "a"}},
	}
	_, err := Ordered(features, []string{"a"})
	if err == nil {
		t.Fatal("expected error for overrideFeatureInstallOrder")
	}
}
