package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strconv"
	"strings"

	photos "github.com/photos-sh/photos-go"

	"github.com/photos-sh/photos-cli/internal/output"
)

// The commands that take images and give data back, the lists, and the
// account. With --json each prints the API's JSON as it came.

const oneImageArgs = "[flags] <image|->"

func num(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func round(f float64) string { return strconv.FormatFloat(f, 'f', 2, 64) }

func findingRows(findings []photos.Finding) [][]string {
	rows := make([][]string, 0, len(findings))
	for _, f := range findings {
		msg := f.Message
		if f.Detail != "" {
			msg += " (" + f.Detail + ")"
		}
		rows = append(rows, []string{"  " + f.Severity, f.Code, msg})
	}
	return rows
}

func checkCommand() *Command {
	var o photos.CheckOptions
	return &Command{
		Name:    "check",
		Group:   groupRead,
		Summary: "measure a photo's quality and check it against a marketplace's rules",
		Args:    oneImageArgs,
		Long: `Measures sharpness, noise, exposure and colour cast. With --marketplace
the photo is also checked against that marketplace's rules. The exit code
says whether the check ran, not what it found: read the verdict from the
output (with --json, .marketplace.passed).`,
		Examples: []string{
			"photos check shoe.jpg",
			"photos check --marketplace amazon --json shoe.jpg | jq '.marketplace.passed'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Marketplace, "marketplace", "", "check against this marketplace's rules: its `name`, such as amazon")
			fs.BoolVar(&o.Subject, "subject", false, "find the product, for framing and background checks")
			fs.BoolVar(&o.Text, "text", false, "read the text in the photo")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.Check(ctx, in, &o)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}

			m := res.Metrics
			rows := [][]string{
				{"Size", fmt.Sprintf("%dx%d", m.Width, m.Height)},
				{"Sharpness", round(m.Sharpness)},
			}
			if m.Noise != nil {
				rows = append(rows, []string{"Noise", round(*m.Noise)})
			}
			rows = append(rows, []string{"Brightness", round(m.Brightness)})
			if s := m.Subject; s != nil {
				rows = append(rows, []string{"Subject", fmt.Sprintf("%dx%d at %d,%d, fills %s of the frame", s.Width, s.Height, s.X, s.Y, round(s.Fill))})
			}
			p.Table(nil, rows)

			if len(res.Findings) == 0 {
				p.Data("Findings: none")
			} else {
				p.Data("Findings:")
				p.Table(nil, findingRows(res.Findings))
			}
			if len(res.Text) > 0 {
				p.Data("Text:")
				for _, t := range res.Text {
					where := ""
					if t.OnProduct != nil && *t.OnProduct {
						where = "  (on the product)"
					}
					p.Data("  %s%s", output.OneLine(t.Text), where)
				}
			}
			if v := res.Marketplace; v != nil {
				verdict := p.Paint(output.Green, "passes")
				if !v.Passed {
					verdict = p.Paint(output.Red, "does not pass")
				}
				p.Data("Marketplace %s: %s", output.OneLine(v.Marketplace), verdict)
				p.Table(nil, findingRows(v.Findings))
				if len(v.NotChecked) > 0 {
					p.Data("  not checked: %s", output.OneLine(strings.Join(v.NotChecked, ", ")))
				}
			}
			return nil
		},
	}
}

func barcodesCommand() *Command {
	var o photos.ReadBarcodesOptions
	return &Command{
		Name:    "barcodes",
		Group:   groupRead,
		Summary: "read the barcodes in a photo",
		Args:    oneImageArgs,
		Long: `Prints one code per line: its format and its value. Retail codes (EAN-13,
UPC-A, ITF) are read by default; --all-formats adds the others.`,
		Examples: []string{
			"photos barcodes box.jpg",
			"photos barcodes --all-formats --json box.jpg | jq -r '.barcodes[].value'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.BoolVar(&o.AllFormats, "all-formats", false, "also read EAN-8, UPC-E, Code 128/39/93, QR, Data Matrix and PDF417")
			fs.StringVar(&o.Engine, "engine", "", "the decoder's `name`: auto, zbar or rxing (default: auto)")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.ReadBarcodes(ctx, in, &o)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			if len(res.Barcodes) == 0 {
				p.Note("No barcodes found.")
				return nil
			}
			rows := make([][]string, 0, len(res.Barcodes))
			for _, b := range res.Barcodes {
				rows = append(rows, []string{b.Format, b.Value})
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func ocrCommand() *Command {
	return &Command{
		Name:    "ocr",
		Group:   groupRead,
		Summary: "read the text in a photo",
		Args:    "[flags] <image|->",
		Long: `Prints the text line by line, top to bottom. With --json each line comes
with its confidence and its corners in pixels.`,
		Examples: []string{
			"photos ocr label.jpg",
			"photos ocr --json label.jpg | jq -r '.lines[] | select(.score > 0.9) | .text'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.ReadText(ctx, in)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			if len(res.Lines) == 0 {
				p.Note("No text found.")
			}
			for _, line := range res.Lines {
				p.Data("%s", output.OneLine(line.Text))
			}
			return nil
		},
	}
}

// labelRows flattens what was read off a label into rows of name and value,
// from the API's own JSON so that a field this version does not know is
// still shown.
func labelRows(raw json.RawMessage) [][]string {
	var doc struct {
		Label map[string]any `json:"label"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	keys := make([]string, 0, len(doc.Label))
	for k := range doc.Label {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var rows [][]string
	for _, k := range keys {
		if v := plain(doc.Label[k]); v != "" {
			rows = append(rows, []string{k, v})
		}
	}
	return rows
}

// plain renders a JSON value on one line for a person.
func plain(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case float64:
		return num(v)
	case bool:
		return strconv.FormatBool(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s := plain(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if s := plain(v[k]); s != "" {
				parts = append(parts, k+" "+s)
			}
		}
		return strings.Join(parts, "; ")
	}
	return fmt.Sprint(v)
}

func labelCommand() *Command {
	return &Command{
		Name:    "label",
		Group:   groupRead,
		Summary: "read product details off a label",
		Args:    "[flags] <image|directory|->...",
		Long: `Takes one to eight photos of ONE product (front, back, side…): text and
codes from all of them are pooled into one answer. Prints what was
understood: the headline, GTIN, net quantity, ingredients, allergens, dates
and so on. A directory stands for the images in it.`,
		Examples: []string{
			"photos label front.jpg back.jpg",
			"photos label --json front.jpg back.jpg | jq -r '.label.gtin'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			inputs, err := someImages(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.ReadLabel(ctx, inputs)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			rows := labelRows(res.Raw)
			if len(rows) == 0 {
				p.Note("Nothing was understood from the label (%d lines of text read; --json shows them).", len(res.Text))
				return nil
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func identifyCommand() *Command {
	var o photos.IdentifyOptions
	return &Command{
		Name:    "identify",
		Group:   groupRead,
		Summary: "match photos of a product against your catalogue",
		Args:    "[flags] <image|directory|->...",
		Long: `Takes one to eight photos of ONE product and prints the catalogue products
that match, best first, with what matched (barcode, sku, ...). The catalogue
is one of your organisation's own, uploaded on the console's Catalogues page.
Catalogue matching is turned on per organisation: until it is on for yours
and a catalogue is uploaded, the API answers "not found".`,
		Examples: []string{
			"photos identify --catalogue shoes front.jpg back.jpg",
			"photos identify --top 1 --json front.jpg | jq -r '.candidates[0].product_id'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Catalogue, "catalogue", "", "the catalogue's `name`; not needed when your organisation has exactly one")
			fs.IntVar(&o.Top, "top", 0, "how many candidates to print, `1-50` (default: 5)")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			inputs, err := someImages(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.Identify(ctx, inputs, &o)
			if errors.Is(err, photos.ErrNotFound) {
				return withHint(err, "Catalogue matching is not turned on for your organisation, or it has no catalogue yet: see the Catalogues page in the Photos.sh console.")
			}
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			if len(res.Candidates) == 0 {
				p.Note("No product in the catalogue matches.")
				return nil
			}
			rows := make([][]string, 0, len(res.Candidates))
			for _, c := range res.Candidates {
				rows = append(rows, []string{round(c.Score), c.ProductID, c.Title, strings.Join(c.Evidence, ", ")})
			}
			p.Table([]string{"SCORE", "PRODUCT", "TITLE", "EVIDENCE"}, rows)
			return nil
		},
	}
}

func paletteCommand() *Command {
	var o photos.PaletteOptions
	return &Command{
		Name:    "palette",
		Group:   groupRead,
		Summary: "list the main colours of a photo",
		Args:    oneImageArgs,
		Long: `Prints one colour per line, the most common first: its hex value, the
share of the image it stands for, and its red, green and blue. Transparent
pixels do not count, so a cutout's palette is the subject's. No model is
involved, and the same image always gives the same colours.`,
		Examples: []string{
			"photos palette shoe.jpg",
			"photos palette --colors 3 --json shoe.jpg | jq -r '.colors[].hex'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.IntVar(&o.Colors, "colors", 0, "how many colours to print at most, `1-16` (default: 5)")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.Palette(ctx, in, &o)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			if len(res.Colors) == 0 {
				p.Note("No colours: nothing in the image can be seen.")
				return nil
			}
			rows := make([][]string, 0, len(res.Colors))
			for _, c := range res.Colors {
				rgb := make([]string, 0, len(c.RGB))
				for _, v := range c.RGB {
					rgb = append(rgb, strconv.Itoa(v))
				}
				rows = append(rows, []string{c.Hex, strconv.FormatFloat(c.Share*100, 'f', 1, 64) + "%", strings.Join(rgb, ",")})
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func facesCommand() *Command {
	var o photos.FindFacesOptions
	return &Command{
		Name:    "faces",
		Group:   groupRead,
		Summary: "find the faces in a photo",
		Args:    oneImageArgs,
		Long: `Prints one face per line, the surest first: where its box is (x,y of the
top-left corner, then width x height, in the photo's pixels after its EXIF
rotation), how sure the model is, and the tilt of the eyes in degrees. With
--json each face also has five landmarks: the eyes, the nose and the corners
of the mouth.

It finds faces; it does not recognise who anyone is. A face must be about
1/60 of the photo's longest side to be found, a profile is found less
surely, and a head seen from behind or mostly covered is not found. No face
found is an answer, not a failure: the exit code stays 0.`,
		Examples: []string{
			"photos faces team.jpg",
			"photos faces --min-score 0.4 --json team.jpg | jq '.faces | length'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.Float64Var(&o.MinScore, "min-score", 0, "how sure the model must be that something is a face, as a `score` from 0.1 to 1 (default: 0.6)")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.FindFaces(ctx, in, &o)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			if len(res.Faces) == 0 {
				p.Note("No faces found.")
				return nil
			}
			whole := func(f float64) string { return strconv.FormatFloat(f, 'f', 0, 64) }
			rows := make([][]string, 0, len(res.Faces))
			for _, face := range res.Faces {
				b := face.Box
				rows = append(rows, []string{
					whole(b.X) + "," + whole(b.Y), whole(b.Width) + "x" + whole(b.Height),
					round(face.Score), strconv.FormatFloat(face.Roll, 'f', 1, 64),
				})
			}
			p.Table([]string{"AT", "SIZE", "SCORE", "ROLL"}, rows)
			return nil
		},
	}
}

func captionCommand() *Command {
	var o photos.CaptionOptions
	return &Command{
		Name:    "caption",
		Group:   groupRead,
		Summary: "describe a photo in words",
		Args:    oneImageArgs,
		Long: `Prints a description of the photo: one sentence, or more with --detail.
The same photo always gets the same words.

The description can be wrong (a colour, a count, what an object is) and is
in English only; it does not know names, brands or places unless they are
written in the photo. Read it before publishing it as alt text or a product
description. The short form is the dependable one: the longer forms say more
and are wrong more often.`,
		Examples: []string{
			"photos caption shoe.jpg",
			"photos caption --detail paragraph --json shoe.jpg | jq -r '.caption'",
		},
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&o.Detail, "detail", "", "the `length` of the description: short (one sentence), detailed (two or three) or paragraph (default: short)")
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			in, err := oneImage(env, args)
			if err != nil {
				return err
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			res, err := client.Caption(ctx, in, &o)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(res.Raw)
			}
			p.Data("%s", strings.TrimSpace(output.Clean(res.Caption)))
			return nil
		},
	}
}

func presetsCommand() *Command {
	return &Command{
		Name:    "presets",
		Group:   groupAccount,
		Summary: "list the built-in presets `photos run` takes",
		Args:    "[flags]",
		Examples: []string{
			"photos presets",
			"photos presets --json | jq -r '.[].name'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("presets takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			presets, err := client.Presets(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				if presets == nil {
					presets = []photos.Preset{}
				}
				return p.JSONLine(presets)
			}
			rows := make([][]string, 0, len(presets))
			for _, preset := range presets {
				rows = append(rows, []string{preset.Name, preset.Description})
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func marketplacesCommand() *Command {
	return &Command{
		Name:    "marketplaces",
		Group:   groupAccount,
		Summary: "list the marketplaces `photos check` and `photos fix` know",
		Args:    "[flags]",
		Long:    "With --json, each marketplace comes with its rules and what `photos fix` produces for it.",
		Examples: []string{
			"photos marketplaces",
			`photos marketplaces --json | jq '.[] | select(.name == "amazon") | .rules'`,
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("marketplaces takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			marketplaces, err := client.Marketplaces(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				raws := make([]json.RawMessage, 0, len(marketplaces))
				for _, m := range marketplaces {
					raws = append(raws, m.Raw)
				}
				return p.JSONLine(raws)
			}
			rows := make([][]string, 0, len(marketplaces))
			for _, m := range marketplaces {
				rows = append(rows, []string{m.Name, m.Title, strconv.Itoa(len(m.Rules)) + " rules", "checked " + m.Checked})
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func idPhotosCommand() *Command {
	return &Command{
		Name:    "id-photos",
		Group:   groupAccount,
		Summary: "list the documents `photos id-photo` knows",
		Args:    "[flags]",
		Long: `Prints each document's name (what --document takes), its photo and head
sizes in millimetres, the background its rules ask for, and the date the
rules were last compared with the authority's own pages. With --json, each
document also comes with those pages, the rules the tool cannot check, and
what the authority says about photos changed by software.`,
		Examples: []string{
			"photos id-photos",
			`photos id-photos --json | jq -r '.[] | select(.name == "us-passport") | .alteration'`,
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("id-photos takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			documents, err := client.IDPhotos(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				raws := make([]json.RawMessage, 0, len(documents))
				for _, d := range documents {
					raws = append(raws, d.Raw)
				}
				return p.JSONLine(raws)
			}
			rows := make([][]string, 0, len(documents))
			for _, d := range documents {
				rows = append(rows, []string{
					d.Name, d.Title,
					num(d.WidthMm) + "x" + num(d.HeightMm) + " mm",
					"head " + num(d.HeadMinMm) + "-" + num(d.HeadMaxMm) + " mm",
					d.Background, "checked " + d.Checked,
				})
			}
			p.Table(nil, rows)
			return nil
		},
	}
}

func accountCommand() *Command {
	return &Command{
		Name:    "account",
		Group:   groupAccount,
		Summary: "show the account, its credits and prices",
		Args:    "[flags]",
		Long: `Shows which account the key belongs to, the credits left and what one
image costs for each operation. The key itself is never printed: only its
last four characters, and where it came from.`,
		Examples: []string{
			"photos account",
			"photos account --json | jq '.credits'",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("account takes no arguments")
			}
			client, err := env.Client()
			if err != nil {
				return err
			}
			acc, err := client.Account(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				return p.Raw(acc.Raw)
			}
			cred, err := env.credential()
			if err != nil {
				return err
			}
			rows := [][]string{}
			if acc.Account != "" {
				rows = append(rows, []string{"Account", acc.Account})
			}
			rows = append(rows, []string{"Key", maskKey(cred.key) + " (" + cred.source + ")"})
			rows = append(rows, []string{"API", env.baseURL(cred)})
			switch {
			case acc.Unlimited:
				rows = append(rows, []string{"Credits", "unlimited"})
			case acc.Credits != nil:
				rows = append(rows, []string{"Credits", strconv.FormatInt(*acc.Credits, 10)})
			}
			p.Table(nil, rows)
			if len(acc.Prices) > 0 {
				ops := make([]string, 0, len(acc.Prices))
				for op := range acc.Prices {
					ops = append(ops, op)
				}
				sort.Strings(ops)
				prices := make([][]string, 0, len(ops))
				for _, op := range ops {
					prices = append(prices, []string{"  " + op, strconv.FormatInt(acc.Prices[op], 10)})
				}
				p.Data("Credits per image:")
				p.Table(nil, prices)
			}
			return nil
		},
	}
}

func healthCommand() *Command {
	return &Command{
		Name:    "health",
		Group:   groupAccount,
		Summary: "say whether the service is ready",
		Args:    "[flags]",
		Long: `Prints ok, loading or failed. Needs no key. The exit code is 0 for ok and
5 otherwise, so a script can wait for the service:

    until photos health --quiet; do sleep 2; done`,
		Examples: []string{
			"photos health",
			"photos health --base-url http://localhost:8080",
		},
		Run: func(ctx context.Context, env *Env, args []string) error {
			if len(args) > 0 {
				return usagef("health takes no arguments")
			}
			// Health sends no key, but the library does not make a client
			// without one: any word will do when there is none.
			key, cred := "none", (*credential)(nil)
			if c, err := env.credential(); err == nil {
				key, cred = c.key, c
			}
			client, err := env.newClient(key, env.baseURL(cred))
			if err != nil {
				return err
			}
			status, err := client.Health(ctx)
			if err != nil {
				return err
			}
			p := env.Printer
			if p.JSON {
				_ = p.JSONLine(map[string]string{"status": status})
			} else if !p.Quiet {
				p.Data("%s", output.OneLine(status))
			}
			if status != photos.HealthOK {
				return &exitError{code: ExitUnavailable}
			}
			return nil
		},
	}
}
