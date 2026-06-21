const fs = require('fs');
const { execSync } = require('child_process');
try {
  const out = execSync('go test -v -cover -count=1 ./internal/collector/...', { encoding: 'utf8' });
  fs.writeFileSync('verify_out.txt', out);
  const runs = (out.match(/=== RUN/g) || []).length;
  const passes = (out.match(/--- PASS/g) || []).length;
  const fails = (out.match(/--- FAIL/g) || []).length;
  const covs = (out.match(/coverage: \d+\.\d+%/g) || []);
  console.log('=== Test Verification ===');
  console.log('Tests run:', runs);
  console.log('Tests passed:', passes);
  console.log('Tests failed:', fails);
  console.log('Coverage statements:', covs);
} catch (e) {
  console.log('Error:', e.message);
  console.log('stdout:', e.stdout?.toString());
  console.log('stderr:', e.stderr?.toString());
}
