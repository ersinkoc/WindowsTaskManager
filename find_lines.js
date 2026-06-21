const fs = require('fs');
const lines = fs.readFileSync('internal/collector/gpu.go', 'utf8').split('\n');
for (let i = 0; i < lines.length; i++) {
  if (lines[i].includes('gpuGetFormatted')) {
    console.log((i + 1) + ': ' + lines[i]);
  }
}
