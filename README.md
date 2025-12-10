# Laninha Tiny PNG

A high-performance Go application for compressing images and videos with parallel processing capabilities. Similar to TinyPNG, but designed for batch processing with multi-threaded execution.

## Overview

Laninha Tiny PNG is a command-line tool that recursively searches through directories and automatically compresses media files to reduce storage space while maintaining visual quality. It processes multiple files in parallel, leveraging all available CPU cores for maximum performance.

## Key Features

- **Parallel Processing**: Multi-threaded execution using worker pools for optimal performance
- **Recursive Search**: Automatically processes all media files in a directory and its subdirectories
- **Image Compression**: Optimizes PNG and JPEG images with quality-preserving algorithms
- **Video Compression**: Compresses videos using H.264 codec with configurable quality settings
- **Automatic Replacement**: Replaces original files with compressed versions (no backup copies)
- **Progress Tracking**: Real-time display of compression statistics and space savings
- **Portable Distribution**: Supports bundled ffmpeg binary for standalone deployment
- **Cross-Platform**: Works on Linux, macOS, and Windows

## Supported Formats

### Images

- PNG (maximum compression)
- JPEG/JPG (85% quality)

### Videos

- MP4, AVI, MOV, MKV, WEBM, FLV, WMV, M4V, 3GP, OGV
- MPG, MPEG, M2V, MTS, M2TS, VOB, ASF, RM, RMVB, DIVX, XVID

## Requirements

- **Go 1.24+** (for building from source)
- **ffmpeg** (required for video compression)

## Installation

### Building from Source

```bash
git clone <repository-url>
cd laninha-tiny-png
make build
```

The compiled binary will be available at `bin/laninha-tiny-png`.

### System-Wide Installation

```bash
make install
```

This installs the binary to `/usr/local/bin/`, making it available system-wide.

## FFmpeg Setup

The application searches for ffmpeg in the following priority order:

1. **Thirdparty folder** (recommended for portable distribution)
2. **System PATH** (fallback)

### Option 1: Bundled FFmpeg (Recommended)

For portable distribution, place the ffmpeg binary in one of these locations:

- `thirdparty/ffmpeg-8.0.1/ffmpeg`
- `thirdparty/ffmpeg/ffmpeg`
- `thirdparty/ffmpeg/bin/ffmpeg`
- `thirdparty/ffmpeg-8.0.1/bin/ffmpeg`

**Linux (Static Build):**

```bash
wget https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz
tar -xf ffmpeg-release-amd64-static.tar.xz
mkdir -p thirdparty/ffmpeg/bin
cp ffmpeg-*-amd64-static/ffmpeg thirdparty/ffmpeg/bin/
chmod +x thirdparty/ffmpeg/bin/ffmpeg
```

**macOS:**

```bash
brew install ffmpeg
mkdir -p thirdparty/ffmpeg/bin
cp $(which ffmpeg) thirdparty/ffmpeg/bin/
```

**Windows:**
Download from [ffmpeg.org](https://ffmpeg.org/download.html) and extract `ffmpeg.exe` to `thirdparty/ffmpeg/bin/`

### Option 2: System FFmpeg

If ffmpeg is not found in the thirdparty folder, the application will use the system-installed version:

**Ubuntu/Debian:**

```bash
sudo apt-get update
sudo apt-get install ffmpeg
```

**macOS:**

```bash
brew install ffmpeg
```

**Windows:**
Download from [ffmpeg.org](https://ffmpeg.org/download.html) or use Chocolatey:

```bash
choco install ffmpeg
```

## Usage

### Basic Usage

```bash
laninha-tiny-png <folder-path>
```

### Examples

```bash
# Compress all media files in current directory
laninha-tiny-png .

# Compress files in a specific folder
laninha-tiny-png ./my-media-folder

# Compress files recursively in a path
laninha-tiny-png /home/user/photos

# Using Makefile
make run ARGS='./folder/path'
```

### Command-Line Options

The application accepts a single argument: the path to the folder containing media files to compress.

## Output Format

The application provides real-time feedback during processing:

```
Searching for media files in: ./media
Supported formats:
  Images: PNG, JPEG, JPG
  Videos: MP4, AVI, MOV, MKV, WEBM, FLV, WMV, M4V, 3GP, OGV
Using ffmpeg: /usr/bin/ffmpeg
Found 150 files to process. Using 16 parallel workers...
✓ [Image] ./media/photo1.png: 1024000 bytes → 512000 bytes (50.0% saved)
✓ [Image] ./media/photo2.jpg: 2048000 bytes → 1536000 bytes (25.0% saved)
✓ [Video] ./media/video1.mp4: 10485760 bytes → 5242880 bytes (50.0% saved)
...

=== Summary ===
Total files processed: 150
  - Images: 120
  - Videos: 30
Space saved: 52428800 bytes (51200.00 KB, 50.00 MB)
```

## Performance

The application uses parallel processing to maximize performance:

- **Worker Pool**: Automatically detects CPU cores and creates an optimal number of workers
- **Atomic Operations**: Uses lock-free atomic operations for statistics tracking
- **Buffered Channels**: Efficient task distribution with optimized buffer sizes
- **I/O Optimization**: Minimizes lock contention by separating I/O operations

Typical performance improvements:

- **4-core CPU**: ~3-4x faster than sequential processing
- **8-core CPU**: ~6-8x faster than sequential processing
- **16-core CPU**: ~12-15x faster than sequential processing

## Compression Settings

### Images

- **PNG**: Maximum compression level (best compression ratio)
- **JPEG**: 85% quality (optimal balance between quality and file size)

### Videos

- **Codec**: H.264 (libx264)
- **Quality**: CRF 28 (constant rate factor - good compression with quality preservation)
- **Audio**: AAC codec at 128k bitrate
- **Preset**: Medium (balanced encoding speed and compression)
- **Optimization**: Faststart flag enabled for web streaming compatibility

## Important Notes

⚠️ **Warning**: The application **permanently replaces** original files with compressed versions. There are no backup copies created. Ensure you have backups of important files before running the compression.

- Original files are overwritten only if compression succeeds
- If compression fails, the original file remains unchanged
- Windows Zone.Identifier files are automatically ignored
- The application skips videos if ffmpeg is not available (with warning)

## Makefile Commands

```bash
make build      # Build the application
make run        # Build and run (requires ARGS='./folder')
make install    # Install system-wide
make clean      # Remove compiled files
make help       # Show available commands
```

## Architecture

### Parallel Processing Model

The application uses a worker pool pattern:

1. **File Discovery**: Recursively walks the directory tree to collect all media files
2. **Task Distribution**: Files are sent to a buffered channel
3. **Worker Pool**: Multiple goroutines process files concurrently
4. **Statistics**: Atomic operations track processing statistics thread-safely
5. **Output**: Mutex-protected I/O ensures clean console output

### Thread Safety

- **Atomic Operations**: Used for counter updates (totalProcessed, imagesProcessed, etc.)
- **Mutex Protection**: Only used for console output to prevent interleaved messages
- **Channel Synchronization**: Buffered channels coordinate worker communication
- **WaitGroup**: Ensures all workers complete before final summary

## Troubleshooting

### FFmpeg Not Found

If you see "ffmpeg not found" warnings:

1. Ensure ffmpeg is installed system-wide, OR
2. Place ffmpeg binary in the `thirdparty/` folder
3. Verify executable permissions: `chmod +x thirdparty/ffmpeg/bin/ffmpeg`

### Permission Errors

If you encounter permission errors:

- Ensure you have read/write permissions for the target directory
- On Linux/macOS, you may need `sudo` for system-wide installation

### Processing Errors

- Individual file errors are logged but don't stop the entire process
- Check error messages for specific file issues
- Ensure sufficient disk space for temporary files

## License

[Add your license information here]

## Contributing

[Add contribution guidelines here]

## Support

For issues, questions, or feature requests, please open an issue on the project repository.
