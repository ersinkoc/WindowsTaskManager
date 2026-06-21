const fs = require('fs');
const data = fs.readFileSync('verbose_test.txt', 'utf8');
const runs = (data.match(/=== RUN/g) || []).length;
const passes = (data.match(/--- PASS/g) || []).length;
const fails = (data.match(/--- FAIL/g) || []).length;
console.log('RUN:', runs, 'PASS:', passes, 'FAIL:', fails);
