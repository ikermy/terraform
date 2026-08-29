package delivery

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"terraform-provider-ai/internal/entity"
)

func TestClusterEntityFromModel(t *testing.T) {
	m := clusterModel{
		ID:       types.StringValue("c1"),
		Name:     types.StringValue("demo"),
		Replicas: types.Int64Value(3),
		Model:    types.StringValue("gpt-mini"),
	}
	e := clusterEntityFromModel(m)
	want := entity.Cluster{ID: "c1", Name: "demo", Replicas: 3, Model: "gpt-mini"}
	if e != want {
		t.Fatalf("expected %+v, got %+v", want, e)
	}
}

func TestClusterModelFromEntity(t *testing.T) {
	e := entity.Cluster{ID: "c1", Name: "demo", Replicas: 3, Model: "gpt-mini"}
	m := clusterModelFromEntity(&e)
	if m.ID.ValueString() != "c1" {
		t.Fatalf("unexpected id: %s", m.ID.ValueString())
	}
	if m.Name.ValueString() != "demo" {
		t.Fatalf("unexpected name: %s", m.Name.ValueString())
	}
	if m.Replicas.ValueInt64() != 3 {
		t.Fatalf("unexpected replicas: %d", m.Replicas.ValueInt64())
	}
	if m.Model.ValueString() != "gpt-mini" {
		t.Fatalf("unexpected model: %s", m.Model.ValueString())
	}
}
