// jest-environment-jsdom doesn't expose Node's TextEncoder/TextDecoder globals,
// but react-router-dom's ESM build references them at import time.
const { TextEncoder, TextDecoder } = require('util');

if (typeof global.TextEncoder === 'undefined') {
    global.TextEncoder = TextEncoder;
}
if (typeof global.TextDecoder === 'undefined') {
    global.TextDecoder = TextDecoder;
}
