package main

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
)

func main() {
	cfg, err := parseCLIArgs(os.Args[1:])
	if err != nil {
		fmt.Println(err)
		printUsage()
		os.Exit(1)
	}

	if _, err := os.Stat(cfg.inputPath); os.IsNotExist(err) {
		fmt.Printf("Error: Path '%s' does not exist.\n", cfg.inputPath)
		os.Exit(1)
	}

	fmt.Printf("Searching for media files in: %s\n", cfg.inputPath)
	fmt.Println("Supported formats:")
	if cfg.convertToWebP {
		fmt.Println("  Images: PNG, JPEG, JPG, GIF, BMP, TIFF, TIF, AVIF, HEIC, HEIF, ICO")
	} else {
		fmt.Println("  Images: PNG, JPEG, JPG")
	}
	fmt.Println("  Videos: MP4, AVI, MOV, MKV, WEBM, FLV, WMV, M4V, 3GP, OGV")

	ffmpegPath, ffmpegAvailable := findFFmpeg()
	if ffmpegAvailable {
		fmt.Printf("Using ffmpeg: %s\n", ffmpegPath)
	} else if cfg.convertToWebP {
		fmt.Println("Error: ffmpeg not found. WebP conversion requires ffmpeg.")
		os.Exit(1)
	} else {
		fmt.Println("⚠ Warning: ffmpeg not found. Videos will be skipped.")
	}

	filesToProcess, err := collectFileTasks(cfg.inputPath, cfg.convertToWebP, ffmpegAvailable)
	if err != nil {
		fmt.Printf("Error collecting files: %v\n", err)
		os.Exit(1)
	}

	if len(filesToProcess) == 0 {
		fmt.Println("No media files found to process.")
		return
	}

	numWorkers := runtime.NumCPU()
	if numWorkers > len(filesToProcess) {
		numWorkers = len(filesToProcess)
	}

	fmt.Printf("Found %d files to process. Using %d parallel workers...\n", len(filesToProcess), numWorkers)

	stats := processFilesParallel(filesToProcess, numWorkers, ffmpegPath, cfg.convertToWebP)

	fmt.Printf("\n=== Summary ===\n")
	totalProcessed := atomic.LoadInt64(&stats.totalProcessed)
	imagesProcessed := atomic.LoadInt64(&stats.imagesProcessed)
	videosProcessed := atomic.LoadInt64(&stats.videosProcessed)
	totalSaved := atomic.LoadInt64(&stats.totalSaved)

	fmt.Printf("Total files processed: %d\n", totalProcessed)
	fmt.Printf("  - Images: %d\n", imagesProcessed)
	fmt.Printf("  - Videos: %d\n", videosProcessed)
	fmt.Printf("Space saved: %d bytes (%.2f KB, %.2f MB)\n",
		totalSaved, float64(totalSaved)/1024, float64(totalSaved)/(1024*1024))
}

type fileTask struct {
	path     string
	fileType string
}

type cliConfig struct {
	inputPath     string
	convertToWebP bool
}

type processingStats struct {
	totalProcessed  int64
	imagesProcessed int64
	videosProcessed int64
	totalSaved      int64
}

func printUsage() {
	fmt.Println("Usage: laninha-tiny-png [--webp] <file-or-folder-path>")
	fmt.Println("Example: laninha-tiny-png ./images")
	fmt.Println("Example: laninha-tiny-png --webp ./images/photo.png")
}

func parseCLIArgs(args []string) (cliConfig, error) {
	var cfg cliConfig
	var paths []string

	for _, arg := range args {
		switch arg {
		case "--webp", "-webp":
			cfg.convertToWebP = true
		case "--help", "-h":
			return cfg, fmt.Errorf("help requested")
		default:
			if strings.HasPrefix(arg, "-") {
				return cfg, fmt.Errorf("unknown option: %s", arg)
			}
			paths = append(paths, arg)
		}
	}

	if len(paths) != 1 {
		return cfg, fmt.Errorf("expected exactly one file or folder path")
	}

	cfg.inputPath = paths[0]
	return cfg, nil
}

func collectFileTasks(inputPath string, convertToWebP bool, ffmpegAvailable bool) ([]fileTask, error) {
	info, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}

	if !info.IsDir() {
		task, ok := taskForPath(inputPath, convertToWebP, ffmpegAvailable)
		if !ok {
			ext := strings.ToLower(filepath.Ext(inputPath))
			if convertToWebP && ext != ".webp" && !isVideo(ext) {
				return []fileTask{{
					path:     inputPath,
					fileType: "image",
				}}, nil
			}
			return nil, nil
		}
		return []fileTask{task}, nil
	}

	var filesToProcess []fileTask
	err = filepath.Walk(inputPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		task, ok := taskForPath(path, convertToWebP, ffmpegAvailable)
		if ok {
			filesToProcess = append(filesToProcess, task)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	if convertToWebP {
		if err := validateWebPOutputPaths(filesToProcess); err != nil {
			return nil, err
		}
	}

	return filesToProcess, nil
}

func taskForPath(path string, convertToWebP bool, ffmpegAvailable bool) (fileTask, bool) {
	if strings.Contains(path, ":Zone.Identifier") {
		return fileTask{}, false
	}

	ext := strings.ToLower(filepath.Ext(path))
	if convertToWebP && isWebPConvertibleImage(ext) {
		return fileTask{
			path:     path,
			fileType: "image",
		}, true
	}

	if isImage(ext) {
		return fileTask{
			path:     path,
			fileType: "image",
		}, true
	}

	if isVideo(ext) && ffmpegAvailable {
		return fileTask{
			path:     path,
			fileType: "video",
		}, true
	}

	if isVideo(ext) {
		fmt.Printf("⚠ [Video] %s: ffmpeg not found. Skipping...\n", path)
	}

	return fileTask{}, false
}

func validateWebPOutputPaths(files []fileTask) error {
	outputs := make(map[string]string)
	for _, file := range files {
		if file.fileType != "image" {
			continue
		}

		outputPath := webPOutputPath(file.path)
		if existingPath, exists := outputs[outputPath]; exists {
			return fmt.Errorf("multiple images would write the same WebP output %s: %s and %s", outputPath, existingPath, file.path)
		}
		outputs[outputPath] = file.path
	}

	return nil
}

func processFilesParallel(files []fileTask, numWorkers int, ffmpegPath string, convertToWebP bool) *processingStats {
	stats := &processingStats{}
	bufferSize := numWorkers * 2
	if bufferSize > len(files) {
		bufferSize = len(files)
	}
	fileChan := make(chan fileTask, bufferSize)
	var wg sync.WaitGroup
	var printMu sync.Mutex

	go func() {
		for _, file := range files {
			fileChan <- file
		}
		close(fileChan)
	}()

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for task := range fileChan {
				var originalSize, compressedSize int64
				var err error
				var fileType string
				outputPath := task.path

				if task.fileType == "image" && convertToWebP {
					originalSize, compressedSize, outputPath, err = convertImageToWebP(task.path, ffmpegPath)
					fileType = "WebP"
				} else if task.fileType == "image" {
					originalSize, compressedSize, err = compressImage(task.path)
					fileType = "Image"
				} else if task.fileType == "video" {
					originalSize, compressedSize, err = compressVideo(task.path, ffmpegPath)
					fileType = "Video"
				}

				if err != nil {
					printMu.Lock()
					fmt.Printf("Error processing %s %s: %v\n", strings.ToLower(fileType), task.path, err)
					printMu.Unlock()
					continue
				}

				atomic.AddInt64(&stats.totalProcessed, 1)
				if task.fileType == "image" {
					atomic.AddInt64(&stats.imagesProcessed, 1)
				} else {
					atomic.AddInt64(&stats.videosProcessed, 1)
				}
				saved := originalSize - compressedSize
				atomic.AddInt64(&stats.totalSaved, saved)

				percentSaved := float64(saved) * 100 / float64(originalSize)
				printMu.Lock()
				if outputPath != task.path {
					fmt.Printf("✓ [%s] %s → %s: %d bytes → %d bytes (%.1f%% saved)\n",
						fileType, task.path, outputPath, originalSize, compressedSize, percentSaved)
				} else {
					fmt.Printf("✓ [%s] %s: %d bytes → %d bytes (%.1f%% saved)\n",
						fileType, task.path, originalSize, compressedSize, percentSaved)
				}
				printMu.Unlock()
			}
		}()
	}

	wg.Wait()

	return stats
}

func compressImage(imagePath string) (int64, int64, error) {
	file, err := os.Open(imagePath)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	originalInfo, err := file.Stat()
	if err != nil {
		return 0, 0, err
	}
	originalSize := originalInfo.Size()

	img, format, err := image.Decode(file)
	if err != nil {
		return 0, 0, fmt.Errorf("error decoding image: %v", err)
	}

	tempPath := imagePath + ".tmp"
	outFile, err := os.Create(tempPath)
	if err != nil {
		return 0, 0, err
	}
	defer outFile.Close()

	var compressedSize int64
	switch format {
	case "png":
		encoder := &png.Encoder{
			CompressionLevel: png.BestCompression,
		}
		err = encoder.Encode(outFile, img)
		if err != nil {
			os.Remove(tempPath)
			return 0, 0, err
		}
	case "jpeg":
		err = jpeg.Encode(outFile, img, &jpeg.Options{Quality: 85})
		if err != nil {
			os.Remove(tempPath)
			return 0, 0, err
		}
	default:
		os.Remove(tempPath)
		return 0, 0, fmt.Errorf("unsupported format: %s", format)
	}

	compressedInfo, err := outFile.Stat()
	if err != nil {
		os.Remove(tempPath)
		return 0, 0, err
	}
	compressedSize = compressedInfo.Size()

	file.Close()
	outFile.Close()

	err = os.Rename(tempPath, imagePath)
	if err != nil {
		os.Remove(tempPath)
		return 0, 0, err
	}

	return originalSize, compressedSize, nil
}

func convertImageToWebP(imagePath string, ffmpegPath string) (int64, int64, string, error) {
	originalInfo, err := os.Stat(imagePath)
	if err != nil {
		return 0, 0, "", err
	}
	originalSize := originalInfo.Size()

	outputPath := webPOutputPath(imagePath)
	if _, err := os.Stat(outputPath); err == nil {
		return 0, 0, "", fmt.Errorf("output file already exists: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return 0, 0, "", err
	}

	tempPath := outputPath + ".tmp.webp"
	args := []string{
		"-i", imagePath,
		"-c:v", "libwebp",
		"-quality", "85",
		"-compression_level", "6",
		"-f", "webp",
		"-y",
		"-loglevel", "error",
		tempPath,
	}
	cmd := exec.Command(ffmpegPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		os.Remove(tempPath)
		errorMsg := stderr.String()
		if errorMsg != "" {
			return 0, 0, "", fmt.Errorf("error executing ffmpeg: %v\nffmpeg output: %s", err, errorMsg)
		}
		return 0, 0, "", fmt.Errorf("error executing ffmpeg: %v", err)
	}

	convertedInfo, err := os.Stat(tempPath)
	if err != nil {
		os.Remove(tempPath)
		return 0, 0, "", err
	}
	convertedSize := convertedInfo.Size()

	if err := os.Rename(tempPath, outputPath); err != nil {
		os.Remove(tempPath)
		return 0, 0, "", err
	}

	if err := os.Remove(imagePath); err != nil {
		return 0, 0, "", err
	}

	return originalSize, convertedSize, outputPath, nil
}

func webPOutputPath(imagePath string) string {
	ext := filepath.Ext(imagePath)
	return strings.TrimSuffix(imagePath, ext) + ".webp"
}

func isImage(ext string) bool {
	imageExts := []string{".png", ".jpg", ".jpeg"}
	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}

func isWebPConvertibleImage(ext string) bool {
	imageExts := []string{
		".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff",
		".avif", ".heic", ".heif", ".ico",
	}
	for _, imgExt := range imageExts {
		if ext == imgExt {
			return true
		}
	}
	return false
}

func isVideo(ext string) bool {
	videoExts := []string{
		".mp4", ".avi", ".mov", ".mkv", ".webm", ".flv",
		".wmv", ".m4v", ".3gp", ".ogv", ".mpg", ".mpeg",
		".m2v", ".mts", ".m2ts", ".vob", ".asf", ".rm",
		".rmvb", ".divx", ".xvid",
	}
	for _, vidExt := range videoExts {
		if ext == vidExt {
			return true
		}
	}
	return false
}

func findFFmpeg() (string, bool) {
	execPath, err := os.Executable()
	if err != nil {
		execPath, _ = os.Getwd()
	}
	execDir := filepath.Dir(execPath)

	basePaths := []string{
		filepath.Join(execDir, "thirdparty", "ffmpeg-8.0.1", "ffmpeg"),
		filepath.Join(execDir, "thirdparty", "ffmpeg", "ffmpeg"),
		filepath.Join(execDir, "thirdparty", "ffmpeg", "bin", "ffmpeg"),
		filepath.Join(execDir, "thirdparty", "ffmpeg-8.0.1", "bin", "ffmpeg"),
		filepath.Join("thirdparty", "ffmpeg-8.0.1", "ffmpeg"),
		filepath.Join("thirdparty", "ffmpeg", "ffmpeg"),
		filepath.Join("thirdparty", "ffmpeg", "bin", "ffmpeg"),
		filepath.Join("thirdparty", "ffmpeg-8.0.1", "bin", "ffmpeg"),
	}

	possiblePaths := make([]string, 0, len(basePaths)*2)
	for _, path := range basePaths {
		possiblePaths = append(possiblePaths, path)
		if runtime.GOOS == "windows" {
			possiblePaths = append(possiblePaths, path+".exe")
		}
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			if isExecutable(path) {
				return path, true
			}
		}
	}

	systemPath, err := exec.LookPath("ffmpeg")
	if err == nil {
		return systemPath, true
	}

	return "", false
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	if runtime.GOOS == "windows" {
		return strings.HasSuffix(strings.ToLower(path), ".exe")
	}

	mode := info.Mode()
	return mode&0111 != 0
}

func getOutputFormat(ext string) string {
	ext = strings.ToLower(ext)
	switch ext {
	case ".mp4", ".m4v":
		return "mp4"
	case ".avi":
		return "avi"
	case ".mov":
		return "mov"
	case ".mkv":
		return "matroska"
	case ".webm":
		return "webm"
	case ".flv":
		return "flv"
	case ".wmv":
		return "asf"
	case ".ogv":
		return "ogg"
	case ".mpg", ".mpeg":
		return "mpeg"
	default:
		return "mp4"
	}
}

func compressVideo(videoPath string, ffmpegPath string) (int64, int64, error) {
	originalInfo, err := os.Stat(videoPath)
	if err != nil {
		return 0, 0, err
	}
	originalSize := originalInfo.Size()

	ext := filepath.Ext(videoPath)
	tempPath := videoPath + ".tmp" + ext
	outputFormat := getOutputFormat(ext)

	args := []string{
		"-i", videoPath,
		"-c:v", "libx264",
		"-crf", "28",
		"-preset", "medium",
		"-c:a", "aac",
		"-b:a", "128k",
		"-y",
		"-loglevel", "error",
	}

	if outputFormat == "mp4" {
		args = append(args, "-movflags", "+faststart")
	}

	if outputFormat != "" {
		args = append(args, "-f", outputFormat)
	}

	args = append(args, tempPath)
	cmd := exec.Command(ffmpegPath, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		os.Remove(tempPath)
		errorMsg := stderr.String()
		if errorMsg != "" {
			return 0, 0, fmt.Errorf("error executing ffmpeg: %v\nffmpeg output: %s", err, errorMsg)
		}
		return 0, 0, fmt.Errorf("error executing ffmpeg: %v", err)
	}

	compressedInfo, err := os.Stat(tempPath)
	if err != nil {
		os.Remove(tempPath)
		return 0, 0, err
	}
	compressedSize := compressedInfo.Size()

	err = os.Rename(tempPath, videoPath)
	if err != nil {
		os.Remove(tempPath)
		return 0, 0, err
	}

	return originalSize, compressedSize, nil
}
