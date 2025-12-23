// DISCLAIMER: Entirely written by Claude Opus 4.5

import {CaptchaSolver} from './CaptchaSolver.js';
import {readFileSync, writeFileSync} from 'fs';
import sharp from 'sharp';

async function testCaptchaSolver() {
  console.log('Loading test captcha data from login_response.json...');
  
  const loginResponse = JSON.parse(readFileSync('./login_response.json', 'utf-8'));
  const captchaData = loginResponse.data.captcha;
  
  console.log(`Captcha data loaded:`);
  console.log(`  - Background image: ${captchaData.background.length} chars (base64)`);
  console.log(`  - Slider image: ${captchaData.slider.length} chars (base64)`);
  console.log(`  - Top offset: ${captchaData.top}`);
  
  // Save images for visual inspection
  const bgBuffer = Buffer.from(captchaData.background, 'base64');
  const sliderBuffer = Buffer.from(captchaData.slider, 'base64');
  
  writeFileSync('/tmp/captcha_background.png', new Uint8Array(bgBuffer));
  writeFileSync('/tmp/captcha_slider.png', new Uint8Array(sliderBuffer));
  console.log('\nSaved images to /tmp/captcha_background.png and /tmp/captcha_slider.png');
  
  // Get image dimensions
  const bgMeta = await sharp(bgBuffer).metadata();
  const sliderMeta = await sharp(sliderBuffer).metadata();
  console.log(`  - Background size: ${bgMeta.width}x${bgMeta.height}`);
  console.log(`  - Slider size: ${sliderMeta.width}x${sliderMeta.height}`);
  
  // Solve the captcha
  console.log('\nSolving captcha...');
  const solver = new CaptchaSolver();
  const startTime = Date.now();
  const solution = await solver.solve(captchaData);
  const duration = Date.now() - startTime;
  
  console.log(`\nSolution found in ${duration}ms:`);
  console.log(`  - Offset: ${solution.offset} pixels`);
  console.log(`  - Trail points: ${solution.trail.x.length}`);
  console.log(`  - Trail X: [${solution.trail.x.join(', ')}]`);
  console.log(`  - Trail Y: [${solution.trail.y.join(', ')}]`);
  
  // Create a visualization by drawing the slider at the solved position
  console.log('\nCreating visualization...');
  
  // Composite the slider onto the background at the solved position
  const composite = await sharp(bgBuffer)
    .composite([{
      input: sliderBuffer,
      left: solution.offset,
      top: captchaData.top
    }])
    .toBuffer();
  
  writeFileSync('/tmp/captcha_solved.png', new Uint8Array(composite));
  console.log('Saved visualization to /tmp/captcha_solved.png');
  
  console.log('\n✅ Test completed successfully!');
  console.log('Please visually inspect /tmp/captcha_solved.png to verify the puzzle piece alignment.');
}

testCaptchaSolver().catch(console.error);
