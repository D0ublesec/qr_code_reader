package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	_ "image/png"
	_ "image/gif"
	"io"
	"log"
	"net/http"
	"os/exec"
	"runtime"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

func main() {
	// Create Gin router
	r := gin.Default()

	// Enable CORS
	r.Use(cors.Default())

	// Load HTML templates
	r.LoadHTMLGlob("templates/*")

	// Main page route
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// QR code decode endpoint
	r.POST("/api/decode", decodeQRCode)

	// Start server
	port := "8080"
	addr := fmt.Sprintf(":%s", port)
	
	log.Printf("Starting QR Code Reader server on http://localhost%s", addr)
	
	// Open browser automatically
	go openBrowser(fmt.Sprintf("http://localhost%s", addr))
	
	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

func decodeQRCode(c *gin.Context) {
	// Get the uploaded file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read file: " + err.Error()})
		return
	}
	defer file.Close()

	// Check file size (max 10MB)
	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum size is 10MB"})
		return
	}

	// Read file into memory
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read file: " + err.Error()})
		return
	}

	// Decode QR code
	result, err := decodeQRCodeFromBytes(fileBytes)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to decode QR code: " + err.Error()})
		return
	}

	// Try to parse as JSON if possible
	var jsonData interface{}
	if json.Unmarshal([]byte(result), &jsonData) == nil {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"content": result,
			"format":  "json",
			"parsed":  jsonData,
		})
	} else {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"content": result,
			"format":  "text",
		})
	}
}

func decodeQRCodeFromBytes(imageBytes []byte) (string, error) {
	// Decode the image using Go's standard image library
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return "", fmt.Errorf("failed to decode image: %w", err)
	}

	// Try multiple preprocessing strategies
	gray := convertToGrayscale(img)
	enhanced := enhanceContrast(gray)
	threshold128 := thresholdImage(gray, 128)
	threshold100 := thresholdImage(gray, 100)
	threshold150 := thresholdImage(gray, 150)
	adaptive := adaptiveThreshold(gray)
	sharpened := sharpenImage(gray)
	
	// Inverted versions (for white-on-dark QR codes like Nametag)
	invertedGray := invertImage(gray)
	invertedEnhanced := invertImage(enhanced)
	invertedThreshold128 := invertImage(threshold128)
	invertedAdaptive := invertImage(adaptive)
	
	preprocessedImages := []image.Image{
		img,                                    // Original
		gray,                                   // Grayscale
		invertedGray,                          // Inverted grayscale (for white-on-dark)
		enhanced,                              // Grayscale + contrast
		invertedEnhanced,                      // Inverted enhanced
		threshold100,                          // Binary threshold (low)
		threshold128,                          // Binary threshold (mid)
		invertedThreshold128,                 // Inverted threshold (for white-on-dark)
		threshold150,                          // Binary threshold (high)
		adaptive,                              // Adaptive threshold
		invertedAdaptive,                      // Inverted adaptive
		sharpened,                             // Sharpened
		thresholdImage(sharpened, 128),        // Sharpened + threshold
		invertImage(thresholdImage(sharpened, 128)), // Inverted sharpened+threshold
	}
	
	// Add scaled versions (try even larger scales for very small QR codes)
	// Prioritize inverted versions since Nametag uses white-on-dark QR codes
	scales := []float64{2.0, 3.0, 4.0, 5.0, 6.0, 8.0, 10.0}
	for _, scale := range scales {
		preprocessedImages = append(preprocessedImages,
			scaleImage(invertedGray, scale),           // Scaled inverted (priority for white-on-dark)
			scaleImage(invertedEnhanced, scale),      // Scaled inverted enhanced
			scaleImage(invertedThreshold128, scale),   // Scaled inverted threshold
			scaleImage(invertedAdaptive, scale),       // Scaled inverted adaptive
			scaleImage(img, scale),                    // Scaled original
			scaleImage(gray, scale),                  // Scaled grayscale
			scaleImage(enhanced, scale),              // Scaled enhanced
			scaleImage(threshold128, scale),          // Scaled threshold
			scaleImage(adaptive, scale),              // Scaled adaptive
			scaleImage(sharpened, scale),             // Scaled sharpened
			scaleImage(thresholdImage(sharpened, 128), scale), // Scaled sharpened+threshold
			scaleImage(invertImage(thresholdImage(sharpened, 128)), scale), // Scaled inverted sharpened+threshold
		)
	}

	// Create QR code reader
	reader := qrcode.NewQRCodeReader()

	// Try multiple decoding strategies
	strategies := []map[gozxing.DecodeHintType]interface{}{
		// Strategy 1: Try harder with character set
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
			gozxing.DecodeHintType_TRY_HARDER: true,
			gozxing.DecodeHintType_CHARACTER_SET: "UTF-8",
		},
		// Strategy 2: Try harder without character set
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
			gozxing.DecodeHintType_TRY_HARDER: true,
		},
		// Strategy 3: Basic attempt
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
		},
		// Strategy 4: Try with inverted image
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
			gozxing.DecodeHintType_TRY_HARDER: true,
			gozxing.DecodeHintType_ALSO_INVERTED: true,
		},
		// Strategy 5: Pure barcode mode (assumes clean binary image)
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
			gozxing.DecodeHintType_PURE_BARCODE: true,
		},
		// Strategy 6: Pure barcode + try harder
		{
			gozxing.DecodeHintType_POSSIBLE_FORMATS: []gozxing.BarcodeFormat{
				gozxing.BarcodeFormat_QR_CODE,
			},
			gozxing.DecodeHintType_PURE_BARCODE: true,
			gozxing.DecodeHintType_TRY_HARDER: true,
		},
	}

	var lastErr error
	var attemptCount int
	// Try each preprocessed image with each strategy using gozxing
	for i, processedImg := range preprocessedImages {
		// Convert image to binary bitmap for gozxing
		bmp, err := gozxing.NewBinaryBitmapFromImage(processedImg)
		if err != nil {
			continue // Skip if bitmap conversion fails
		}

		// Try with QR code reader
		for j, hints := range strategies {
			attemptCount++
			result, err := reader.Decode(bmp, hints)
			if err == nil {
				log.Printf("Successfully decoded QR code after %d attempts (preprocessing #%d, strategy #%d)", 
					attemptCount, i+1, j+1)
				return result.GetText(), nil
			}
			lastErr = err
		}
	}
	
	log.Printf("Tried %d preprocessing combinations × %d strategies = %d total attempts", 
		len(preprocessedImages), len(strategies), attemptCount)

	// Optional: If gozxing failed, you can add goqr as a fallback
	// Install with: GOPROXY=direct go get github.com/procommerz/goqr
	// Then uncomment the tryGoQR call below

	// If all strategies failed, return a helpful error message
	if lastErr != nil {
		return "", fmt.Errorf("could not detect QR code in image. Tried multiple libraries and preprocessing. Make sure the image contains a clear QR code. Last error: %w", lastErr)
	}

	return "", fmt.Errorf("failed to decode QR code: no valid QR code found in image")
}

// Optional: Uncomment this function after installing goqr library
// Install with: GOPROXY=direct go get github.com/procommerz/goqr
/*
import "github.com/procommerz/goqr"

func tryGoQR(imageBytes []byte) (string, error) {
	// Decode the image
	img, _, err := image.Decode(bytes.NewReader(imageBytes))
	if err != nil {
		return "", fmt.Errorf("goqr: failed to decode image: %w", err)
	}

	// Helper function to try recognizing
	tryRecognize := func(img image.Image) (string, error) {
		codes, err := goqr.Recognize(img)
		if err != nil {
			return "", err
		}
		if len(codes) > 0 {
			return codes[0].Payload, nil
		}
		return "", fmt.Errorf("no QR code found")
	}

	// Try with original image
	if result, err := tryRecognize(img); err == nil {
		return result, nil
	}

	// Try with grayscale
	gray := convertToGrayscale(img)
	if result, err := tryRecognize(gray); err == nil {
		return result, nil
	}

	// Try with enhanced contrast
	enhanced := enhanceContrast(gray)
	if result, err := tryRecognize(enhanced); err == nil {
		return result, nil
	}

	// Try with threshold
	threshold := thresholdImage(gray, 128)
	if result, err := tryRecognize(threshold); err == nil {
		return result, nil
	}

	// Try with adaptive threshold
	adaptive := adaptiveThreshold(gray)
	if result, err := tryRecognize(adaptive); err == nil {
		return result, nil
	}

	// Try scaled versions
	for _, scale := range []float64{2.0, 3.0, 4.0, 5.0} {
		scaled := scaleImage(gray, scale)
		if result, err := tryRecognize(scaled); err == nil {
			return result, nil
		}

		scaledThreshold := scaleImage(threshold, scale)
		if result, err := tryRecognize(scaledThreshold); err == nil {
			return result, nil
		}

		scaledAdaptive := scaleImage(adaptive, scale)
		if result, err := tryRecognize(scaledAdaptive); err == nil {
			return result, nil
		}
	}

	return "", fmt.Errorf("goqr: could not decode QR code")
}
*/

// convertToGrayscale converts an image to grayscale
func convertToGrayscale(img image.Image) image.Image {
	bounds := img.Bounds()
	gray := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			gray.Set(x, y, img.At(x, y))
		}
	}
	
	return gray
}

// enhanceContrast enhances the contrast of a grayscale image
func enhanceContrast(img image.Image) image.Image {
	bounds := img.Bounds()
	enhanced := image.NewGray(bounds)
	
	// Find min and max values for contrast stretching
	minVal := uint8(255)
	maxVal := uint8(0)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			if c.Y < minVal {
				minVal = c.Y
			}
			if c.Y > maxVal {
				maxVal = c.Y
			}
		}
	}
	
	// Apply contrast stretching
	rangeVal := float64(maxVal - minVal)
	if rangeVal == 0 {
		rangeVal = 1 // Avoid division by zero
	}
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			// Stretch contrast to full range
			newVal := uint8(float64(c.Y-minVal) * 255.0 / rangeVal)
			enhanced.Set(x, y, color.Gray{Y: newVal})
		}
	}
	
	return enhanced
}

// thresholdImage converts a grayscale image to pure black and white using a threshold
func thresholdImage(img image.Image, threshold uint8) image.Image {
	bounds := img.Bounds()
	binary := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			if c.Y > threshold {
				binary.Set(x, y, color.Gray{Y: 255}) // White
			} else {
				binary.Set(x, y, color.Gray{Y: 0}) // Black
			}
		}
	}
	
	return binary
}

// adaptiveThreshold applies adaptive thresholding to create a binary image
func adaptiveThreshold(img image.Image) image.Image {
	bounds := img.Bounds()
	binary := image.NewGray(bounds)
	blockSize := 15 // Size of neighborhood for adaptive threshold
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			// Calculate local mean
			var sum uint32
			count := 0
			
			startY := y - blockSize/2
			if startY < bounds.Min.Y {
				startY = bounds.Min.Y
			}
			endY := y + blockSize/2
			if endY >= bounds.Max.Y {
				endY = bounds.Max.Y - 1
			}
			
			startX := x - blockSize/2
			if startX < bounds.Min.X {
				startX = bounds.Min.X
			}
			endX := x + blockSize/2
			if endX >= bounds.Max.X {
				endX = bounds.Max.X - 1
			}
			
			for yy := startY; yy <= endY; yy++ {
				for xx := startX; xx <= endX; xx++ {
					c := color.GrayModel.Convert(img.At(xx, yy)).(color.Gray)
					sum += uint32(c.Y)
					count++
				}
			}
			
			localMean := uint8(sum / uint32(count))
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			
			// Use local mean - 10 as threshold
			threshold := localMean
			if threshold > 10 {
				threshold -= 10
			}
			
			if c.Y > threshold {
				binary.Set(x, y, color.Gray{Y: 255}) // White
			} else {
				binary.Set(x, y, color.Gray{Y: 0}) // Black
			}
		}
	}
	
	return binary
}

// invertImage inverts the colors of an image (black becomes white, white becomes black)
func invertImage(img image.Image) image.Image {
	bounds := img.Bounds()
	inverted := image.NewGray(bounds)
	
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
			// Invert: 255 - original value
			inverted.Set(x, y, color.Gray{Y: 255 - c.Y})
		}
	}
	
	return inverted
}

// sharpenImage applies a sharpening filter to enhance edges
func sharpenImage(img image.Image) image.Image {
	bounds := img.Bounds()
	sharpened := image.NewGray(bounds)
	
	// Sharpening kernel
	kernel := [][]float64{
		{0, -1, 0},
		{-1, 5, -1},
		{0, -1, 0},
	}
	
	for y := bounds.Min.Y + 1; y < bounds.Max.Y-1; y++ {
		for x := bounds.Min.X + 1; x < bounds.Max.X-1; x++ {
			var sum float64
			for ky := -1; ky <= 1; ky++ {
				for kx := -1; kx <= 1; kx++ {
					c := color.GrayModel.Convert(img.At(x+kx, y+ky)).(color.Gray)
					sum += float64(c.Y) * kernel[ky+1][kx+1]
				}
			}
			// Clamp to valid range
			if sum < 0 {
				sum = 0
			}
			if sum > 255 {
				sum = 255
			}
			sharpened.Set(x, y, color.Gray{Y: uint8(sum)})
		}
	}
	
	// Copy edges
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if y == bounds.Min.Y || y == bounds.Max.Y-1 || x == bounds.Min.X || x == bounds.Max.X-1 {
				c := color.GrayModel.Convert(img.At(x, y)).(color.Gray)
				sharpened.Set(x, y, c)
			}
		}
	}
	
	return sharpened
}

// scaleImage scales an image by the given factor using nearest neighbor
func scaleImage(img image.Image, factor float64) image.Image {
	bounds := img.Bounds()
	newWidth := int(float64(bounds.Dx()) * factor)
	newHeight := int(float64(bounds.Dy()) * factor)
	
	// Use Gray for grayscale images, RGBA for color
	if _, ok := img.(*image.Gray); ok {
		scaled := image.NewGray(image.Rect(0, 0, newWidth, newHeight))
		for y := 0; y < newHeight; y++ {
			for x := 0; x < newWidth; x++ {
				srcX := bounds.Min.X + int(float64(x)/factor)
				srcY := bounds.Min.Y + int(float64(y)/factor)
				
				if srcX < bounds.Max.X && srcY < bounds.Max.Y {
					scaled.Set(x, y, img.At(srcX, srcY))
				}
			}
		}
		return scaled
	}
	
	scaled := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
	for y := 0; y < newHeight; y++ {
		for x := 0; x < newWidth; x++ {
			srcX := bounds.Min.X + int(float64(x)/factor)
			srcY := bounds.Min.Y + int(float64(y)/factor)
			
			if srcX < bounds.Max.X && srcY < bounds.Max.Y {
				scaled.Set(x, y, img.At(srcX, srcY))
			}
		}
	}
	
	return scaled
}

func openBrowser(url string) {
	// Wait a moment for the server to start
	time.Sleep(1 * time.Second)

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		log.Printf("Please open your browser and navigate to: %s", url)
		return
	}

	if err := cmd.Start(); err != nil {
		log.Printf("Failed to open browser automatically. Please open: %s", url)
	}
}
