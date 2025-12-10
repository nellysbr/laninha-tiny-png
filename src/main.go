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
	"sync"
	"sync/atomic"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: laninha-tiny-png <folder-path>")
		fmt.Println("Example: laninha-tiny-png ./images")
		os.Exit(1)
	}

	folderPath := os.Args[1]

	if _, err := os.Stat(folderPath); os.IsNotExist(err) {
		fmt.Printf("Error: Folder '%s' does not exist.\n", folderPath)
		os.Exit(1)
	}

	fmt.Printf("Searching for media files in: %s\n", folderPath)
	fmt.Println("Supported formats:")
	fmt.Println("  Images: PNG, JPEG, JPG")
	fmt.Println("  Videos: MP4, AVI, MOV, MKV, WEBM, FLV, WMV, M4V, 3GP, OGV")

	ffmpegPath, ffmpegAvailable := findFFmpeg()
	if ffmpegAvailable {
		fmt.Printf("Using ffmpeg: %s\n", ffmpegPath)
	} else {
		fmt.Println("⚠ Warning: ffmpeg not found. Videos will be skipped.")
	}

	var filesToProcess []fileTask
	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if strings.Contains(path, ":Zone.Identifier") {
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))

		if isImage(ext) {
			filesToProcess = append(filesToProcess, fileTask{
				path: path,
				fileType: "image",
			})
		} else if isVideo(ext) {
			if ffmpegAvailable {
				filesToProcess = append(filesToProcess, fileTask{
					path: path,
					fileType: "video",
				})
			} else {
				fmt.Printf("⚠ [Video] %s: ffmpeg not found. Skipping...\n", path)
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking folder: %v\n", err)
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

	stats := processFilesParallel(filesToProcess, numWorkers, ffmpegPath)

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

type processingStats struct {
	totalProcessed  int64
	imagesProcessed int64
	videosProcessed int64
	totalSaved      int64
}

func processFilesParallel(files []fileTask, numWorkers int, ffmpegPath string) *processingStats {
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

				if task.fileType == "image" {
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
				fmt.Printf("✓ [%s] %s: %d bytes → %d bytes (%.1f%% saved)\n",
					fileType, task.path, originalSize, compressedSize, percentSaved)
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

func isImage(ext string) bool {
	imageExts := []string{".png", ".jpg", ".jpeg"}
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
