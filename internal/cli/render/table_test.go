package render_test

import (
	"bytes"
	"errors"
	"regexp"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/cli/render"
)

func TestRenderTable_IndicatorStylingPreservesAlignment(
	t *testing.T,
) {
	rows := [][]render.Cell{
		{{
			Text: "alpha",
		}, {
			Text:      "healthy",
			Indicator: "◉",
			Color:     render.Green,
			Bold:      true,
		}},
		{{
			Text: "beta",
		}, {
			Text: "pending",
		}},
	}
	var plain, styled bytes.Buffer
	for _, tc := range []struct {
		out   *bytes.Buffer
		style bool
	}{{
		&plain,
		false,
	}, {
		&styled,
		true,
	}} {
		if err := (render.Table{
			Headers: []string{"NAME", "STATUS"},
			Rows:    rows,
		}).Render(tc.out, render.Options{
			Style: tc.style,
		}); err != nil {
			t.Fatal(err)
		}
	}
	want := "NAME   STATUS\nalpha  ◉ healthy\nbeta     pending\n"
	if plain.String() != want {
		t.Fatalf("table:\n%s\nwant:\n%s", plain.String(), want)
	}
	ansi := regexp.MustCompile(`\x1b\[[0-9;]*m`)
	if ansi.ReplaceAllString(styled.String(), "") != want {
		t.Fatalf("styled table changes alignment: %q", styled.String())
	}
	if !strings.Contains(styled.String(), "\x1b[1m\x1b[32m◉\x1b[39m healthy\x1b[22m") {
		t.Fatalf("indicator color must end before text without clearing bold: %q", styled.String())
	}
}

func TestRenderTable_RejectsExtraCellsBeforeWriting(
	t *testing.T,
) {
	var out bytes.Buffer
	err := (render.Table{
		Headers: []string{"NAME"},
		Rows:    [][]render.Cell{render.Cells("one", "extra")},
	}).Render(&out, render.Options{})
	if err == nil || out.Len() != 0 {
		t.Fatalf("err=%v, output=%q", err, out.String())
	}
}

var errWrite = errors.New("write failed")

type failingWriter struct{}

func (failingWriter) Write(
	[]byte,
) (
	int,
	error,
) {
	return 0, errWrite
}

func TestRenderTable_PropagatesWriterError(
	t *testing.T,
) {
	err := (render.Table{
		Headers: []string{"NAME"},
		Rows:    nil,
	}).Render(failingWriter{}, render.Options{})
	if !errors.Is(err, errWrite) {
		t.Fatalf("error=%v", err)
	}
}

func TestRenderTable_KeepsCellsOnOneLine(
	t *testing.T,
) {
	var out bytes.Buffer
	err := (render.Table{
		Headers: []string{"NAME"},
		Rows:    [][]render.Cell{render.Cells("line one\nline two\tend")},
	}).Render(&out, render.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "NAME\nline one; line two end\n" {
		t.Fatalf("output=%q", got)
	}
}
