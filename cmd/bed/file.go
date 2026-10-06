package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// fileDocument keeps disk representation separate from textarea's normalized
// text. Tabs use an unused private-use rune because textarea expands literal
// tabs on input. The view renders this marker as ⇥; save restores real tabs.
type fileDocument struct {
	path     string
	original []byte
	exists   bool
	newline  string
	tab      rune
}

func openDocument(path string) (fileDocument, string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return fileDocument{}, "", err
	}
	if info, err := os.Lstat(absolute); err == nil && info.Mode()&os.ModeSymlink != 0 {
		absolute, err = filepath.EvalSymlinks(absolute)
		if err != nil {
			return fileDocument{}, "", err
		}
	}
	d := fileDocument{path: absolute, newline: "\n"}
	data, err := os.ReadFile(absolute)
	if err != nil && !os.IsNotExist(err) {
		return d, "", err
	}
	d.exists = err == nil
	d.original = data
	if !utf8.Valid(data) {
		return d, "", fmt.Errorf("only UTF-8 text files are supported")
	}
	text := string(data)
	if strings.Contains(text, "\r\n") {
		if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\n") {
			return d, "", fmt.Errorf("mixed LF and CRLF line endings are not supported")
		}
		d.newline = "\r\n"
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	for _, r := range text {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || r == utf8.RuneError {
			return d, "", fmt.Errorf("unsupported text character U+%04X", r)
		}
	}
	for r := rune(0xE000); r <= 0xF8FF; r++ {
		if !strings.ContainsRune(text, r) {
			d.tab = r
			break
		}
	}
	if d.tab == 0 {
		return d, "", fmt.Errorf("no available tab marker")
	}
	return d, strings.ReplaceAll(text, "\t", string(d.tab)), nil
}
func (d fileDocument) decode(text string) []byte {
	text = strings.ReplaceAll(text, string(d.tab), "\t")
	return []byte(strings.ReplaceAll(text, "\n", d.newline))
}
func (d fileDocument) encodeInput(text string) (string, error) {
	if strings.ContainsRune(text, d.tab) {
		return "", fmt.Errorf("input contains the reserved tab marker")
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	for _, r := range text {
		if (unicode.IsControl(r) && r != '\n' && r != '\t') || r == utf8.RuneError {
			return "", fmt.Errorf("input contains unsupported text character U+%04X", r)
		}
	}
	return strings.ReplaceAll(text, "\t", string(d.tab)), nil
}

func (d *fileDocument) save(text string) error {
	data := d.decode(text)
	current, err := os.ReadFile(d.path)
	if d.exists {
		if err != nil {
			return fmt.Errorf("check file before saving: %w", err)
		}
		if !bytes.Equal(current, d.original) {
			return fmt.Errorf("file changed on disk; save cancelled")
		}
	} else if err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("file appeared or cannot be checked; save cancelled")
	}
	// Avoid replacing a newly introduced symlink or special file.
	mode := os.FileMode(0o600)
	if d.exists {
		info, err := os.Lstat(d.path)
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("target is no longer a regular file")
		}
		mode = info.Mode().Perm()
	}
	f, err := os.CreateTemp(filepath.Dir(d.path), ".bed-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(name, d.path); err != nil {
		return err
	}
	d.original = data
	d.exists = true
	return nil
}
