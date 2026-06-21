const c = require('./coverage/coverage-final.json');
const keys = Object.keys(c).filter(k => k.includes('search-input') || k.includes('confirm-dialog') || k.includes('detail-tile') || k.includes('page-header') || k.includes('test-utils') || k.includes('use-debounced-value'));
keys.forEach(k => console.log(k));
console.log('---');
keys.forEach(k => {
  const v = c[k];
  console.log(k, 'stmts:', v.s, 'branches:', v.b, 'funcs:', v.f, 'lines:', v.l);
});
