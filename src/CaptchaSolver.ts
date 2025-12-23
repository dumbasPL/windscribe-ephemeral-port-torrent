// DISCLAIMER: Entirely written by Claude Opus 4.5

import sharp from 'sharp';

/**
 * Captcha solution containing the offset and fake mouse trail
 */
export interface CaptchaSolution {
  offset: number;
  trail: {
    x: number[];
    y: number[];
  };
}

/**
 * Captcha data received from Windscribe auth endpoint
 */
export interface CaptchaData {
  background: string;  // Base64 encoded PNG
  slider: string;      // Base64 encoded PNG (puzzle piece)
  top: number;         // Y offset where the piece should be placed
}

const PIXELS_EXTENSION = 10;

/**
 * Solver for Windscribe slider captcha using edge detection and template matching.
 * Based on the approach from https://github.com/peduajo/geetest-slice-captcha-solver
 */
export class CaptchaSolver {

  /**
   * Solve a slider captcha by finding the correct X offset
   * @param captchaData The captcha data containing background, slider, and top offset
   * @returns The solution with offset and fake mouse trail
   */
  async solve(captchaData: CaptchaData): Promise<CaptchaSolution> {
    // Decode base64 images
    const backgroundBuffer = Buffer.from(captchaData.background, 'base64');
    const sliderBuffer = Buffer.from(captchaData.slider, 'base64');

    // Get image dimensions
    const bgMetadata = await sharp(backgroundBuffer).metadata();
    const sliderMetadata = await sharp(sliderBuffer).metadata();

    if (!bgMetadata.width || !bgMetadata.height || !sliderMetadata.width || !sliderMetadata.height) {
      throw new Error('Failed to get image dimensions');
    }

    // Apply Sobel edge detection to both images
    const backgroundEdges = await this.applySobelOperator(backgroundBuffer);
    const sliderEdges = await this.applySobelOperator(sliderBuffer);

    // Crop the slider to just the puzzle piece (remove transparent areas)
    const { croppedTemplate, cropX, cropY, cropHeight } = await this.cropPuzzlePiece(
      sliderEdges,
      sliderMetadata.width,
      sliderMetadata.height
    );

    // Calculate the Y range in the background to search (based on top offset and piece height)
    const yStart = Math.max(0, captchaData.top + cropY - PIXELS_EXTENSION);
    const yEnd = Math.min(bgMetadata.height, captchaData.top + cropHeight + PIXELS_EXTENSION);

    // Extract the relevant horizontal strip from background
    const backgroundStrip = await this.extractRegion(
      backgroundEdges,
      bgMetadata.width,
      bgMetadata.height,
      0,
      yStart,
      bgMetadata.width,
      yEnd - yStart
    );

    // Perform template matching
    const offset = await this.templateMatch(
      backgroundStrip,
      bgMetadata.width,
      yEnd - yStart,
      croppedTemplate.data,
      croppedTemplate.width,
      croppedTemplate.height
    );

    // The offset found is relative to the left edge, adjust for the crop offset
    const finalOffset = offset - cropX + PIXELS_EXTENSION;

    // Generate a human-like mouse trail
    const trail = this.generateHumanTrail(finalOffset, bgMetadata.width);

    console.log(`Captcha solved: offset=${finalOffset}, trail points=${trail.x.length}`);

    return {
      offset: Math.round(finalOffset),
      trail
    };
  }

  /**
   * Apply Sobel edge detection operator to an image
   * Returns grayscale edge-detected image as raw pixel buffer
   */
  private async applySobelOperator(imageBuffer: Buffer): Promise<{
    data: Buffer;
    width: number;
    height: number;
  }> {
    // First convert to grayscale and get raw pixels
    const image = sharp(imageBuffer);
    const metadata = await image.metadata();
    
    if (!metadata.width || !metadata.height) {
      throw new Error('Failed to get image metadata');
    }

    // Apply a slight blur to reduce noise, then extract grayscale raw data
    const grayscaleBuffer = await sharp(imageBuffer)
      .grayscale()
      .blur(0.5) // Gaussian blur with sigma 0.5
      .raw()
      .toBuffer();

    const width = metadata.width;
    const height = metadata.height;

    // Sobel kernels
    const sobelX = [
      [-1, 0, 1],
      [-2, 0, 2],
      [-1, 0, 1]
    ];

    const sobelY = [
      [-1, -2, -1],
      [0, 0, 0],
      [1, 2, 1]
    ];

    // Apply Sobel operator
    const result = Buffer.alloc(width * height);

    for (let y = 1; y < height - 1; y++) {
      for (let x = 1; x < width - 1; x++) {
        let gx = 0;
        let gy = 0;

        // Convolve with Sobel kernels
        for (let ky = -1; ky <= 1; ky++) {
          for (let kx = -1; kx <= 1; kx++) {
            const pixel = grayscaleBuffer[(y + ky) * width + (x + kx)];
            gx += pixel * sobelX[ky + 1][kx + 1];
            gy += pixel * sobelY[ky + 1][kx + 1];
          }
        }

        // Compute gradient magnitude
        const magnitude = Math.sqrt(gx * gx + gy * gy);
        result[y * width + x] = Math.min(255, Math.round(magnitude * 0.5));
      }
    }

    return { data: result, width, height };
  }

  /**
   * Crop the puzzle piece to remove transparent/empty areas
   * Returns the cropped template and the crop coordinates
   */
  private async cropPuzzlePiece(
    edgeData: { data: Buffer; width: number; height: number },
    originalWidth: number,
    originalHeight: number
  ): Promise<{
    croppedTemplate: { data: Buffer; width: number; height: number };
    cropX: number;
    cropY: number;
    cropWidth: number;
    cropHeight: number;
  }> {
    const { data, width, height } = edgeData;

    // Find bounding box of non-zero pixels
    let minX = width, maxX = 0, minY = height, maxY = 0;
    const threshold = 10; // Minimum edge intensity to consider

    for (let y = 0; y < height; y++) {
      for (let x = 0; x < width; x++) {
        if (data[y * width + x] > threshold) {
          minX = Math.min(minX, x);
          maxX = Math.max(maxX, x);
          minY = Math.min(minY, y);
          maxY = Math.max(maxY, y);
        }
      }
    }

    // Add some padding
    minX = Math.max(0, minX - 2);
    maxX = Math.min(width - 1, maxX + 2);
    minY = Math.max(0, minY - 2);
    maxY = Math.min(height - 1, maxY + 2);

    const cropWidth = maxX - minX + 1;
    const cropHeight = maxY - minY + 1;

    // Extract the cropped region
    const croppedData = Buffer.alloc(cropWidth * cropHeight);
    for (let y = 0; y < cropHeight; y++) {
      for (let x = 0; x < cropWidth; x++) {
        croppedData[y * cropWidth + x] = data[(y + minY) * width + (x + minX)];
      }
    }

    // Add border extension (like the Python solver does)
    const extendedWidth = cropWidth + 2 * PIXELS_EXTENSION;
    const extendedHeight = cropHeight + 2 * PIXELS_EXTENSION;
    const extendedData = Buffer.alloc(extendedWidth * extendedHeight);

    // Copy cropped data into center of extended buffer
    for (let y = 0; y < cropHeight; y++) {
      for (let x = 0; x < cropWidth; x++) {
        extendedData[(y + PIXELS_EXTENSION) * extendedWidth + (x + PIXELS_EXTENSION)] = 
          croppedData[y * cropWidth + x];
      }
    }

    return {
      croppedTemplate: { data: extendedData, width: extendedWidth, height: extendedHeight },
      cropX: minX,
      cropY: minY,
      cropWidth,
      cropHeight
    };
  }

  /**
   * Extract a region from the edge-detected image
   */
  private async extractRegion(
    edgeData: { data: Buffer; width: number; height: number },
    fullWidth: number,
    fullHeight: number,
    x: number,
    y: number,
    width: number,
    height: number
  ): Promise<Buffer> {
    const result = Buffer.alloc(width * height);
    
    for (let row = 0; row < height; row++) {
      for (let col = 0; col < width; col++) {
        const srcY = y + row;
        const srcX = x + col;
        if (srcY >= 0 && srcY < fullHeight && srcX >= 0 && srcX < fullWidth) {
          result[row * width + col] = edgeData.data[srcY * edgeData.width + srcX];
        }
      }
    }

    return result;
  }

  /**
   * Perform normalized cross-correlation template matching
   * Returns the X offset where the template best matches
   */
  private async templateMatch(
    background: Buffer,
    bgWidth: number,
    bgHeight: number,
    template: Buffer,
    templateWidth: number,
    templateHeight: number
  ): Promise<number> {
    let maxCorrelation = -Infinity;
    let bestX = 0;

    // Only search in the right portion of the image (puzzle piece slot is typically on the right)
    // Start from ~10% of width to avoid matching at the piece's original position
    const searchStartX = Math.floor(bgWidth * 0.1);
    const searchEndX = bgWidth - templateWidth;

    // Template statistics (precompute for efficiency)
    let templateMean = 0;
    let templateCount = 0;
    for (let i = 0; i < template.length; i++) {
      if (template[i] > 0) {
        templateMean += template[i];
        templateCount++;
      }
    }
    templateMean = templateCount > 0 ? templateMean / templateCount : 0;

    let templateStdDev = 0;
    for (let i = 0; i < template.length; i++) {
      if (template[i] > 0) {
        const diff = template[i] - templateMean;
        templateStdDev += diff * diff;
      }
    }
    templateStdDev = Math.sqrt(templateStdDev / Math.max(1, templateCount));

    // Slide the template across the background
    for (let x = searchStartX; x < searchEndX; x++) {
      // Only search vertically where it makes sense (template should fit)
      const searchStartY = 0;
      const searchEndY = Math.max(0, bgHeight - templateHeight);

      for (let y = searchStartY; y <= searchEndY; y++) {
        // Compute normalized cross-correlation at this position
        let correlation = 0;
        let bgMean = 0;
        let bgCount = 0;

        // First pass: compute background region mean
        for (let ty = 0; ty < templateHeight; ty++) {
          for (let tx = 0; tx < templateWidth; tx++) {
            const bgIdx = (y + ty) * bgWidth + (x + tx);
            if (bgIdx >= 0 && bgIdx < background.length) {
              bgMean += background[bgIdx];
              bgCount++;
            }
          }
        }
        bgMean = bgCount > 0 ? bgMean / bgCount : 0;

        // Second pass: compute correlation
        let bgStdDev = 0;
        let crossCorr = 0;
        for (let ty = 0; ty < templateHeight; ty++) {
          for (let tx = 0; tx < templateWidth; tx++) {
            const tIdx = ty * templateWidth + tx;
            const bgIdx = (y + ty) * bgWidth + (x + tx);
            
            if (bgIdx >= 0 && bgIdx < background.length && tIdx < template.length) {
              const tVal = template[tIdx] - templateMean;
              const bgVal = background[bgIdx] - bgMean;
              crossCorr += tVal * bgVal;
              bgStdDev += bgVal * bgVal;
            }
          }
        }
        bgStdDev = Math.sqrt(bgStdDev / Math.max(1, bgCount));

        // Normalized correlation
        const denom = templateStdDev * bgStdDev * Math.max(1, templateCount);
        correlation = denom > 0 ? crossCorr / denom : 0;

        if (correlation > maxCorrelation) {
          maxCorrelation = correlation;
          bestX = x;
        }
      }
    }

    return bestX;
  }

  /**
   * Generate a human-like mouse trail from 0 to the target offset
   * Simulates natural human mouse movement with slight variations
   */
  private generateHumanTrail(targetOffset: number, canvasWidth: number): { x: number[]; y: number[] } {
    const x: number[] = [];
    const y: number[] = [];

    // Parameters for human-like movement
    const totalDuration = 500 + Math.random() * 300; // 500-800ms total movement
    const numPoints = 15 + Math.floor(Math.random() * 10); // 15-25 points
    
    // Starting position (at the slider button)
    let currentX = 0;
    let currentY = 0;

    // Generate movement points with easing and slight randomness
    for (let i = 0; i <= numPoints; i++) {
      const progress = i / numPoints;
      
      // Use ease-out cubic for more natural deceleration at the end
      const easedProgress = 1 - Math.pow(1 - progress, 3);
      
      // Add slight overshoot near the end (humans often overshoot slightly)
      let targetX = targetOffset * easedProgress;
      if (progress > 0.8 && progress < 0.95) {
        targetX += (Math.random() - 0.3) * 5; // Slight overshoot
      }
      
      // Add random micro-movements
      const jitterX = (Math.random() - 0.5) * 3;
      const jitterY = (Math.random() - 0.5) * 8; // More vertical variation

      currentX = Math.max(0, Math.min(canvasWidth, targetX + jitterX));
      currentY = jitterY;

      // Only record every few movements to simulate the sampling in the real captcha
      if (i % 2 === 0 || i === numPoints) {
        x.push(Math.round(currentX));
        y.push(Math.round(currentY));
      }
    }

    // Ensure the last point is close to the target
    if (x.length > 0) {
      x[x.length - 1] = Math.round(targetOffset);
      y[y.length - 1] = Math.round((Math.random() - 0.5) * 4);
    }

    return { x, y };
  }
}

/**
 * Convenience function to solve a captcha
 */
export async function solveCaptcha(captchaData: CaptchaData): Promise<CaptchaSolution> {
  const solver = new CaptchaSolver();
  return solver.solve(captchaData);
}
