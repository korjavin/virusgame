const assert = require('assert');
const fs = require('fs');
const path = require('path');

const root = path.resolve(__dirname, '..');
const read = (file) => fs.readFileSync(path.join(root, file), 'utf8');

const html = read('index.html');
assert.match(html, /id="rows-input" value="12" min="5" max="50"/);
assert.match(html, /id="cols-input" value="12" min="5" max="50"/);

const multiplayer = read('multiplayer.js');
assert.match(multiplayer, /rowsInput \? parseInt\(rowsInput\.value\) \|\| 12 : 12/);
assert.match(multiplayer, /colsInput \? parseInt\(colsInput\.value\) \|\| 12 : 12/);

const lobby = read('lobby.js');
assert.match(lobby, /parseInt\(document\.getElementById\('rows-input'\)\.value\) \|\| 12/);
assert.match(lobby, /parseInt\(document\.getElementById\('cols-input'\)\.value\) \|\| 12/);

const localGame = read('script.js');
assert.match(localGame, /rows = parseInt\(rowsInput\.value\) \|\| 12/);
assert.match(localGame, /cols = parseInt\(colsInput\.value\) \|\| 12/);

console.log('default board size frontend checks passed');
