package db

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

const (
	maxSkillPackageUncompressed = 4 << 20 // 4 MiB total
	maxSkillZipBytes            = 8 << 20 // 8 MiB archive
)

// NormalizeSkillPackagePaths strips a single shared top-level directory when every
// entry lives under it (common for zip/folder uploads of my-skill/SKILL.md).
func NormalizeSkillPackagePaths(files []SkillFileRecord) ([]SkillFileRecord, error) {
	if len(files) == 0 {
		return nil, fmt.Errorf("skill package is empty")
	}
	cleaned := make([]SkillFileRecord, 0, len(files))
	for _, f := range files {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		p = strings.TrimPrefix(p, "./")
		if p == "" || strings.HasSuffix(p, "/") {
			continue
		}
		cleaned = append(cleaned, SkillFileRecord{Path: p, Content: f.Content})
	}
	if len(cleaned) == 0 {
		return nil, fmt.Errorf("skill package has no files")
	}

	prefix := ""
	for _, f := range cleaned {
		parts := strings.Split(f.Path, "/")
		if len(parts) < 2 {
			prefix = ""
			break
		}
		if prefix == "" {
			prefix = parts[0]
			continue
		}
		if parts[0] != prefix {
			prefix = ""
			break
		}
	}
	if prefix != "" {
		out := make([]SkillFileRecord, 0, len(cleaned))
		for _, f := range cleaned {
			rel := strings.TrimPrefix(f.Path, prefix+"/")
			out = append(out, SkillFileRecord{Path: rel, Content: f.Content})
		}
		cleaned = out
	}

	byPath := map[string]SkillFileRecord{}
	for _, f := range cleaned {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		base := filepath.Base(p)
		if skillFileSkip(base) || strings.EqualFold(base, ".DS_Store") {
			continue
		}
		if !ValidSkillRelPath(p) {
			return nil, fmt.Errorf("invalid path in package: %q", f.Path)
		}
		if len(f.Content) > maxSkillFileBytes {
			return nil, fmt.Errorf("file too large: %q (max %d bytes)", p, maxSkillFileBytes)
		}
		if strings.ContainsRune(f.Content, 0) {
			return nil, fmt.Errorf("binary file not allowed: %q", p)
		}
		if _, ok := byPath[p]; ok {
			return nil, fmt.Errorf("duplicate path: %q", p)
		}
		byPath[p] = SkillFileRecord{Path: p, Content: f.Content}
	}
	if len(byPath) == 0 {
		return nil, fmt.Errorf("skill package has no allowed files")
	}
	if len(byPath) > maxSkillFiles {
		return nil, fmt.Errorf("too many files in skill package (max %d)", maxSkillFiles)
	}
	out := make([]SkillFileRecord, 0, len(byPath))
	total := 0
	for _, f := range byPath {
		total += len(f.Content)
		if total > maxSkillPackageUncompressed {
			return nil, fmt.Errorf("skill package too large (max %d bytes uncompressed)", maxSkillPackageUncompressed)
		}
		out = append(out, f)
	}
	return out, nil
}

// ValidateSkillPackage ensures SKILL.md exists with name + description (Agent Skills).
// nameHint / descHint fill missing frontmatter; name must be ValidSkillName.
func ValidateSkillPackage(files []SkillFileRecord, nameHint, descHint string) (name, desc string, normalized []SkillFileRecord, err error) {
	normalized, err = NormalizeSkillPackagePaths(files)
	if err != nil {
		return "", "", nil, err
	}
	var skillMD string
	for _, f := range normalized {
		if f.Path == "SKILL.md" {
			skillMD = f.Content
			break
		}
	}
	if strings.TrimSpace(skillMD) == "" {
		return "", "", nil, fmt.Errorf("package must include SKILL.md at package root")
	}
	fmName, fmDesc := parseSkillFrontmatter(skillMD, "")
	name = strings.TrimSpace(strings.ToLower(fmName))
	if name == "" {
		name = strings.TrimSpace(strings.ToLower(nameHint))
	}
	desc = strings.TrimSpace(fmDesc)
	if desc == "" {
		desc = strings.TrimSpace(descHint)
	}
	if !ValidSkillName(name) {
		return "", "", nil, fmt.Errorf("invalid skill name %q: use lowercase letters, digits, hyphens", name)
	}
	if desc == "" {
		return "", "", nil, fmt.Errorf("skill description required (SKILL.md frontmatter or form field)")
	}
	md := BuildSkillMarkdown(name, desc, skillMD)
	_, desc = parseSkillFrontmatter(md, name)
	for i := range normalized {
		if normalized[i].Path == "SKILL.md" {
			normalized[i].Content = md
		}
	}
	return name, desc, normalized, nil
}

// ParseZipSkillPackage extracts a skill package from a zip archive with zip-slip and size checks.
func ParseZipSkillPackage(archive []byte) ([]SkillFileRecord, error) {
	if len(archive) == 0 {
		return nil, fmt.Errorf("empty zip")
	}
	if len(archive) > maxSkillZipBytes {
		return nil, fmt.Errorf("zip too large (max %d bytes)", maxSkillZipBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip: %w", err)
	}
	var out []SkillFileRecord
	var total int64
	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		if strings.HasSuffix(name, "/") {
			continue
		}
		base := filepath.Base(name)
		if strings.EqualFold(base, ".DS_Store") || strings.HasPrefix(base, ".") {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, fmt.Errorf("unsupported zip compression for %q", name)
		}
		if f.UncompressedSize64 > uint64(maxSkillFileBytes) {
			return nil, fmt.Errorf("zip entry too large: %q", name)
		}
		total += int64(f.UncompressedSize64)
		if total > maxSkillPackageUncompressed {
			return nil, fmt.Errorf("zip uncompressed size exceeds limit")
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("cannot read zip entry %q: %w", name, err)
		}
		data, err := io.ReadAll(io.LimitReader(rc, int64(maxSkillFileBytes)+1))
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		if len(data) > maxSkillFileBytes {
			return nil, fmt.Errorf("zip entry too large: %q", name)
		}
		out = append(out, SkillFileRecord{Path: name, Content: string(data)})
		if len(out) > maxSkillFiles {
			return nil, fmt.Errorf("too many files in zip (max %d)", maxSkillFiles)
		}
	}
	return out, nil
}

// BuildZipSkillPackage packs files under a top-level folder named after the skill.
func BuildZipSkillPackage(skillName string, files []SkillFileRecord) ([]byte, error) {
	skillName = strings.TrimSpace(strings.ToLower(skillName))
	if !ValidSkillName(skillName) {
		return nil, fmt.Errorf("invalid skill name")
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to export")
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	seen := map[string]bool{}
	for _, f := range files {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		if !ValidSkillRelPath(p) {
			_ = zw.Close()
			return nil, fmt.Errorf("invalid path: %q", f.Path)
		}
		if seen[p] {
			continue
		}
		seen[p] = true
		w, err := zw.Create(skillName + "/" + p)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := io.WriteString(w, f.Content); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
