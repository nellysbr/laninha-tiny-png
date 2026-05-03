package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestParseCLIArgsAcceptsWebPFlag(t *testing.T) {
	cfg, err := parseCLIArgs([]string{"--webp", "./images"})
	if err != nil {
		t.Fatalf("parseCLIArgs returned error: %v", err)
	}

	if !cfg.convertToWebP {
		t.Fatal("expected convertToWebP to be true")
	}

	if cfg.inputPath != "./images" {
		t.Fatalf("expected input path ./images, got %q", cfg.inputPath)
	}
}

func TestCollectFileTasksAcceptsSingleImageFile(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(imagePath, []byte("not decoded during discovery"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := collectFileTasks(imagePath, false, false)
	if err != nil {
		t.Fatalf("collectFileTasks returned error: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	if tasks[0].path != imagePath || tasks[0].fileType != "image" {
		t.Fatalf("unexpected task: %#v", tasks[0])
	}
}

func TestCollectFileTasksAcceptsUnknownSingleFileForWebPConversion(t *testing.T) {
	dir := t.TempDir()
	imagePath := filepath.Join(dir, "camera-export.raw")
	if err := os.WriteFile(imagePath, []byte("ffmpeg decides whether this is decodable"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := collectFileTasks(imagePath, true, true)
	if err != nil {
		t.Fatalf("collectFileTasks returned error: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	if tasks[0].path != imagePath || tasks[0].fileType != "image" {
		t.Fatalf("unexpected task: %#v", tasks[0])
	}
}

func TestCollectFileTasksKeepsSingleVideoAsVideoInWebPMode(t *testing.T) {
	dir := t.TempDir()
	videoPath := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(videoPath, []byte("not decoded during discovery"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks, err := collectFileTasks(videoPath, true, true)
	if err != nil {
		t.Fatalf("collectFileTasks returned error: %v", err)
	}

	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}

	if tasks[0].path != videoPath || tasks[0].fileType != "video" {
		t.Fatalf("unexpected task: %#v", tasks[0])
	}
}

func TestCollectFileTasksAcceptsCommonFormatsForWebPConversion(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		filepath.Join(dir, "animation.gif"),
		filepath.Join(dir, "scan.tiff"),
	}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("not decoded during discovery"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tasks, err := collectFileTasks(dir, true, true)
	if err != nil {
		t.Fatalf("collectFileTasks returned error: %v", err)
	}

	if len(tasks) != len(paths) {
		t.Fatalf("expected %d tasks, got %d: %#v", len(paths), len(tasks), tasks)
	}
}

func TestCollectFileTasksRejectsDuplicateWebPOutputs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"photo.png", "photo.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("not decoded during discovery"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	_, err := collectFileTasks(dir, true, true)
	if err == nil {
		t.Fatal("expected duplicate WebP output error")
	}
	if !strings.Contains(err.Error(), "same WebP output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebPOutputPathReplacesExtension(t *testing.T) {
	got := webPOutputPath(filepath.Join("tmp", "photo.large.png"))
	want := filepath.Join("tmp", "photo.large.webp")
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestConvertImageToWebPRemovesOriginalAfterSuccessfulConversion(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake ffmpeg shell script is Unix-only")
	}

	dir := t.TempDir()
	imagePath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(imagePath, []byte("original image"), 0o644); err != nil {
		t.Fatal(err)
	}

	ffmpegPath := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\nout=\"\"\nfor arg in \"$@\"; do\n  out=\"$arg\"\ndone\nprintf webp > \"$out\"\n"
	if err := os.WriteFile(ffmpegPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	originalSize, convertedSize, outputPath, err := convertImageToWebP(imagePath, ffmpegPath)
	if err != nil {
		t.Fatalf("convertImageToWebP returned error: %v", err)
	}

	if originalSize != int64(len("original image")) {
		t.Fatalf("expected original size %d, got %d", len("original image"), originalSize)
	}
	if convertedSize != int64(len("webp")) {
		t.Fatalf("expected converted size %d, got %d", len("webp"), convertedSize)
	}
	if outputPath != filepath.Join(dir, "photo.webp") {
		t.Fatalf("unexpected output path: %q", outputPath)
	}
	if _, err := os.Stat(imagePath); !os.IsNotExist(err) {
		t.Fatalf("expected original image to be removed, stat err: %v", err)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("expected converted image to exist: %v", err)
	}
}
