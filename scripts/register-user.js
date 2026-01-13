#!/usr/bin/env node
// register-user.js - Registers a user in Vaultwarden/Bitwarden using correct encryption
//
// Usage: node register-user.js <server_url> <email> <password> <name>
//
// The Bitwarden API requires:
// 1. PBKDF2-SHA256 password hashing with correct iterations
// 2. Properly encrypted symmetric key

const crypto = require('crypto');
const https = require('https');
const http = require('http');

const KDF_ITERATIONS = 600000; // Bitwarden default for new accounts

function pbkdf2(password, salt, iterations, keyLen) {
    return new Promise((resolve, reject) => {
        crypto.pbkdf2(password, salt, iterations, keyLen, 'sha256', (err, key) => {
            if (err) reject(err);
            else resolve(key);
        });
    });
}

function hkdfExpand(prk, info, length) {
    // Simple HKDF-Expand for SHA256
    const hashLen = 32;
    const n = Math.ceil(length / hashLen);
    let okm = Buffer.alloc(0);
    let t = Buffer.alloc(0);

    for (let i = 1; i <= n; i++) {
        const infoBuffer = Buffer.isBuffer(info) ? info : Buffer.from(info, 'utf8');
        t = crypto.createHmac('sha256', prk)
            .update(Buffer.concat([t, infoBuffer, Buffer.from([i])]))
            .digest();
        okm = Buffer.concat([okm, t]);
    }
    return okm.slice(0, length);
}

async function makeKeys(password, email) {
    const salt = email.toLowerCase();

    // Step 1: Derive master key from password using email as salt
    const masterKey = await pbkdf2(password, salt, KDF_ITERATIONS, 32);

    // Step 2: Derive stretched master key using HKDF
    const encKeyInfo = Buffer.from('enc', 'utf8');
    const macKeyInfo = Buffer.from('mac', 'utf8');

    const stretchedEncKey = hkdfExpand(masterKey, encKeyInfo, 32);
    const stretchedMacKey = hkdfExpand(masterKey, macKeyInfo, 32);

    // Step 3: Hash the master key for authentication (1 iteration with password as salt)
    const masterPasswordHash = await pbkdf2(masterKey, password, 1, 32);

    return {
        masterKey,
        stretchedEncKey,
        stretchedMacKey,
        masterPasswordHash: masterPasswordHash.toString('base64')
    };
}

function encryptSymmetricKey(symmetricKey, encKey, macKey) {
    // Generate random IV
    const iv = crypto.randomBytes(16);

    // Encrypt using AES-256-CBC
    const cipher = crypto.createCipheriv('aes-256-cbc', encKey, iv);
    let ciphertext = cipher.update(symmetricKey);
    ciphertext = Buffer.concat([ciphertext, cipher.final()]);

    // Calculate MAC
    const mac = crypto.createHmac('sha256', macKey)
        .update(Buffer.concat([iv, ciphertext]))
        .digest();

    // Format: encType.iv|ciphertext|mac (all base64)
    // encType 2 = AesCbc256_HmacSha256_B64
    return `2.${iv.toString('base64')}|${ciphertext.toString('base64')}|${mac.toString('base64')}`;
}

async function registerUser(serverUrl, email, password, name) {
    const keys = await makeKeys(password, email);

    // Generate a random 512-bit symmetric key (64 bytes = 32 enc + 32 mac)
    const symmetricKey = crypto.randomBytes(64);

    // Encrypt the symmetric key with stretched master key
    const encryptedKey = encryptSymmetricKey(symmetricKey, keys.stretchedEncKey, keys.stretchedMacKey);

    // Generate RSA key pair for user (optional but recommended)
    // For simplicity, we'll skip this for testing

    const payload = JSON.stringify({
        name: name,
        email: email,
        masterPasswordHash: keys.masterPasswordHash,
        masterPasswordHint: null,
        key: encryptedKey,
        kdf: 0,  // PBKDF2
        kdfIterations: KDF_ITERATIONS,
        referenceId: null,
        captchaResponse: null,
        keys: null  // RSA keys - optional for basic vault usage
    });

    const url = new URL('/api/accounts/register', serverUrl);
    const isHttps = url.protocol === 'https:';
    const client = isHttps ? https : http;

    return new Promise((resolve, reject) => {
        const options = {
            hostname: url.hostname,
            port: url.port || (isHttps ? 443 : 80),
            path: url.pathname,
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'Content-Length': Buffer.byteLength(payload)
            }
        };

        const req = client.request(options, (res) => {
            let data = '';
            res.on('data', chunk => data += chunk);
            res.on('end', () => {
                if (res.statusCode >= 200 && res.statusCode < 300) {
                    resolve({ success: true, statusCode: res.statusCode, data });
                } else {
                    resolve({ success: false, statusCode: res.statusCode, data });
                }
            });
        });

        req.on('error', reject);
        req.write(payload);
        req.end();
    });
}

async function main() {
    const args = process.argv.slice(2);
    if (args.length < 4) {
        console.error('Usage: node register-user.js <server_url> <email> <password> <name>');
        process.exit(1);
    }

    const [serverUrl, email, password, name] = args;

    try {
        console.log(`Registering user ${email} at ${serverUrl}...`);
        const result = await registerUser(serverUrl, email, password, name);

        if (result.success) {
            console.log('Registration successful!');
            process.exit(0);
        } else {
            console.error(`Registration failed with status ${result.statusCode}: ${result.data}`);
            // Exit 0 if user already exists (status 400 with existing message)
            if (result.statusCode === 400 && result.data.includes('already exists')) {
                console.log('User already exists, continuing...');
                process.exit(0);
            }
            process.exit(1);
        }
    } catch (error) {
        console.error('Error:', error.message);
        process.exit(1);
    }
}

main();
