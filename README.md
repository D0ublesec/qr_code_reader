# QR Code Reader

A simple Go web application that allows users to upload QR code images and decode their contents in the browser.

## Features

- 🖥️ Web-based interface
- 📱 Drag & drop file upload
- 🔍 Automatic QR code decoding
- 📄 JSON formatting for structured data
- 🎨 Modern, responsive UI
- 🌐 Automatically opens in browser

## Requirements

- Go 1.21 or higher

## Installation

1. Navigate to the project directory:
```bash
cd qr-reader
```

2. Install dependencies:
```bash
go mod download
```

## Usage

1. Run the application:
```bash
go run main.go
```

**Note:** If you encounter TLS certificate errors or missing go.sum entries, run with:
```bash
GOPROXY=direct go run main.go
```

Alternatively, to fix the go.sum file first:
```bash
GOPROXY=direct go mod download
GOPROXY=direct go mod tidy
```

2. The application will automatically open in your default browser at `http://localhost:8080`

3. Upload a QR code image by:
   - Clicking the upload area and selecting a file
   - Dragging and dropping an image file onto the upload area

4. The decoded QR code content will be displayed below

## Supported Image Formats

- PNG
- JPEG/JPG
- GIF
- Other common image formats supported by the Go image library

## API Endpoint

- `POST /api/decode` - Upload an image file to decode QR code
  - Form data: `file` (image file)
  - Response: JSON with `success`, `content`, `format`, and optionally `parsed` fields

## Project Structure

```
qr-reader/
├── main.go           # Go web server
├── go.mod            # Go dependencies
├── templates/        # HTML templates
│   └── index.html    # Main web page
└── README.md         # This file
```

## Dependencies

- `github.com/gin-gonic/gin` - Web framework
- `github.com/gin-contrib/cors` - CORS middleware
- `github.com/makiuchi-d/gozxing` - QR code decoding library
