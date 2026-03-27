/**
 * Tests for AuthContext.
 *
 * Verifies:
 * 1. signIn does NOT log token to console -- WILL FAIL
 * 2. User object does not contain hardcoded Devias template data -- WILL FAIL
 */

const fs = require('fs');
const path = require('path');

const source = fs.readFileSync(
  path.join(__dirname, '..', 'auth-context.js'),
  'utf8'
);

test('auth-context should not log token to console', () => {
  // Check for console.log calls that reference "token"
  // auth-context.js:100 has: console.log("AUTH CONTEXT --> auth.user.token:", ...)
  const lines = source.split('\n');
  const tokenLogLines = lines.filter(
    (l) =>
      l.includes('console.log') &&
      l.includes('token') &&
      !l.trim().startsWith('//')
  );

  if (tokenLogLines.length > 0) {
    throw new Error(
      'BUG: auth-context.js logs token to browser console -- ' +
        'exposes credentials in DevTools.\n' +
        'Lines: ' +
        tokenLogLines.map((l) => l.trim()).join('\n')
    );
  }
});

test('user object should not contain hardcoded Devias template data', () => {
  if (source.includes('5e86809283e28b96d2d38537')) {
    throw new Error(
      'BUG: auth-context.js contains hardcoded Devias user ID "5e86809283e28b96d2d38537"'
    );
  }
  if (source.includes('anika.visser@devias.io')) {
    throw new Error(
      'BUG: auth-context.js contains hardcoded Devias email "anika.visser@devias.io"'
    );
  }
});
