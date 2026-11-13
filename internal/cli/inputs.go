package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	photos "github.com/photos-sh/photos-go"
)

// imageExtensions are the files a directory is searched for: the formats the
// API reads. A file named on the command line is sent whatever it is called.
var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".webp": true, ".gif": true,
	".tif": true, ".tiff": true, ".bmp": true, ".heic": true, ".heif": true, ".avif": true,
}

// source is one image to send.
type source struct {
	// display is how it is named in the output: the path as given, or "-".
	display string
	// path is the file; empty for standard input.
	path string
	// subdir is, for a file found in a directory, where it was below that
	// directory, so --out-dir can keep the layout.
	subdir string
	// data is standard input, read up front so a retry can send it again.
	data  []byte
	stdin bool
	// err is why the argument cannot be used (no such file); it fails this
	// file and not the others.
	err error
}

// input is the source for the library.
func (s source) input() photos.Input {
	if s.stdin {
		return photos.Bytes("stdin", s.data)
	}
	return photos.File(s.path)
}

// stem is the file's name without its extension.
func (s source) stem() string {
	if s.stdin {
		return "stdin"
	}
	base := filepath.Base(s.path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ext is the file's extension in lower case, "" for standard input.
func (s source) ext() string {
	return strings.ToLower(filepath.Ext(s.path))
}

func isURL(arg string) bool {
	return strings.HasPrefix(arg, "http://") || strings.HasPrefix(arg, "https://")
}

// inputOptions say how arguments become sources.
type inputOptions struct {
	// recursive searches directories below the ones named.
	recursive bool
	// urls allows http(s) URLs (batch jobs); they are returned apart.
	urls bool
	// skipSuffix leaves out files in a directory whose name ends with this
	// before the extension: results of an earlier run beside their sources.
	skipSuffix string
	// skipDir leaves out this directory when searching: the output
	// directory, when it is inside the input.
	skipDir string
}

// expandInputs turns the arguments of a command into the images to send:
// files as named, directories searched for images (sorted by name), and "-"
// for standard input.
func expandInputs(env *Env, args []string, opts inputOptions) (sources []source, urls []string, err error) {
	stdinSeen := false
	for _, arg := range args {
		switch {
		case arg == "-":
			if stdinSeen {
				return nil, nil, usagef("standard input (-) can be given once")
			}
			stdinSeen = true
			if env.stdinIsTerminal() {
				return nil, nil, usagef("- reads an image from standard input, which is the keyboard: pipe one in, or name a file")
			}
			data, rerr := io.ReadAll(env.In)
			if rerr != nil {
				return nil, nil, fmt.Errorf("reading standard input: %w", rerr)
			}
			s := source{display: "-", stdin: true, data: data}
			if len(data) == 0 {
				s.err = errors.New("standard input is empty")
			}
			sources = append(sources, s)

		case isURL(arg):
			if !opts.urls {
				return nil, nil, usagef("%s is a URL: this command takes files; `photos jobs create` takes URLs", arg)
			}
			urls = append(urls, arg)

		case arg == "":
			return nil, nil, usagef("an empty file name")

		default:
			info, serr := os.Stat(arg)
			switch {
			case serr != nil:
				sources = append(sources, source{display: arg, path: arg, err: pathError(serr)})
			case info.IsDir():
				found, werr := searchDir(env, arg, opts)
				if werr != nil {
					return nil, nil, werr
				}
				sources = append(sources, found...)
			default:
				sources = append(sources, source{display: arg, path: arg})
			}
		}
	}
	return sources, urls, nil
}

// pathError is a file error without the "stat" in front.
func pathError(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// searchDir lists the images in a directory, sorted, below it too when
// recursive. Hidden files and directories are left out.
func searchDir(env *Env, dir string, opts inputOptions) ([]source, error) {
	var found []source
	skipped := 0
	skipDir := ""
	if opts.skipDir != "" {
		skipDir, _ = filepath.Abs(opts.skipDir)
	}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}
		hidden := strings.HasPrefix(d.Name(), ".")
		if d.IsDir() {
			if !opts.recursive || hidden {
				return filepath.SkipDir
			}
			if abs, _ := filepath.Abs(path); skipDir != "" && abs == skipDir {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if hidden || !imageExtensions[ext] {
			return nil
		}
		// Follow a link to a file; leave out anything that is not a file.
		if info, serr := os.Stat(path); serr != nil || !info.Mode().IsRegular() {
			return nil
		}
		if opts.skipSuffix != "" && strings.HasSuffix(strings.TrimSuffix(d.Name(), filepath.Ext(d.Name())), opts.skipSuffix) {
			skipped++
			return nil
		}
		subdir, _ := filepath.Rel(dir, filepath.Dir(path))
		if subdir == "." {
			subdir = ""
		}
		found = append(found, source{display: path, path: path, subdir: subdir})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("searching %s: %w", dir, pathError(err))
	}
	sort.SliceStable(found, func(i, j int) bool { return found[i].path < found[j].path })

	if skipped > 0 {
		env.Printer.Note("%s: left out %d file(s) named *%s.*: results of an earlier run", dir, skipped, opts.skipSuffix)
	}
	if len(found) == 0 {
		how := ""
		if !opts.recursive {
			how = " (--recursive searches below it)"
		}
		env.Printer.Warn("%s: no images in this directory%s", dir, how)
	}
	return found, nil
}

// oneImage is the single image of a command that takes exactly one.
func oneImage(env *Env, args []string) (photos.Input, error) {
	if len(args) != 1 {
		return photos.Input{}, usagef("give one image: a file, or - for standard input")
	}
	if info, err := os.Stat(args[0]); err == nil && info.IsDir() {
		return photos.Input{}, usagef("%s is a directory: give one image", args[0])
	}
	sources, _, err := expandInputs(env, args, inputOptions{})
	if err != nil {
		return photos.Input{}, err
	}
	if sources[0].err != nil {
		return photos.Input{}, fmt.Errorf("%s: %w", sources[0].display, sources[0].err)
	}
	return sources[0].input(), nil
}

// someImages is the images of a command that takes several of one product
// (label, identify): files, a directory of them, or standard input.
func someImages(env *Env, args []string) ([]photos.Input, error) {
	if len(args) == 0 {
		return nil, usagef("give one to %d images of the product", photos.MaxImagesPerRequest)
	}
	sources, _, err := expandInputs(env, args, inputOptions{})
	if err != nil {
		return nil, err
	}
	if len(sources) == 0 {
		return nil, errors.New("no images to send")
	}
	if len(sources) > photos.MaxImagesPerRequest {
		return nil, usagef("%d images: at most %d photos of one product", len(sources), photos.MaxImagesPerRequest)
	}
	inputs := make([]photos.Input, 0, len(sources))
	for _, s := range sources {
		if s.err != nil {
			return nil, fmt.Errorf("%s: %w", s.display, s.err)
		}
		inputs = append(inputs, s.input())
	}
	return inputs, nil
}
