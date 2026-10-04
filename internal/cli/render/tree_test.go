package render_test

import (
	"bytes"
	"reflect"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
)

func TestRenderTree_PreservesForestOrderAndInput(
	t *testing.T,
) {
	roots := []render.Node{
		{
			Cells: render.Cells("first", "a"),
			Children: []render.Node{
				{
					Cells: render.Cells("child", "b"),
					Children: []render.Node{{
						Cells: render.Cells("leaf", "c"),
					}},
				},
				{
					Cells: render.Cells("last", "d"),
				},
			},
		},
		{
			Cells: render.Cells("second", "e"),
		},
	}
	var out bytes.Buffer
	if err := (render.Tree{
		Headers: []string{"NODE", "VALUE"},
		Roots:   roots,
	}).Render(&out, render.Options{}); err != nil {
		t.Fatal(err)
	}
	want := "NODE        VALUE\nfirst       a\n├─ child    b\n│  └─ leaf  c\n└─ last     d\nsecond      e\n"
	if out.String() != want {
		t.Fatalf("tree:\n%s\nwant:\n%s", out.String(), want)
	}
	if !reflect.DeepEqual(roots[0].Children[0].Cells, render.Cells("child", "b")) {
		t.Fatal("render changed source cells")
	}
	out.Reset()
	if err := (render.Tree{
		Headers: []string{"NODE", "VALUE"},
		Roots:   roots,
	}).Render(&out, render.Options{}); err != nil {
		t.Fatal(err)
	}
	if out.String() != want {
		t.Fatal("render is not repeatable")
	}
}
