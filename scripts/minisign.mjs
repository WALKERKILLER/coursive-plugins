// minisign.mjs — Ed25519 signatures in minisign's format for index.json.
// Signatures use minisign's "Ed" algorithm, which signs the file itself, so
// `minisign -V -m index.json -p minisign.pub` verifies them and Coursive's
// daemon needs nothing beyond Ed25519.
// The key ID is the first 8 bytes of the public key's SHA-256.
import { createHash, createPrivateKey, createPublicKey, sign, verify } from 'node:crypto';

const decode = (line, size, what) => {
  const b = Buffer.from(line ?? '', 'base64');
  if (b.length !== size || b.subarray(0, 2).toString() !== 'Ed') throw new Error(`${what} is not a minisign Ed25519 ${what}`);
  return b;
};

// signMinisign signs message with a base64 PKCS#8 Ed25519 key.
export function signMinisign(message, pkcs8Base64, trusted) {
  const key = createPrivateKey({ key: Buffer.from(pkcs8Base64, 'base64'), format: 'der', type: 'pkcs8' });
  const pk = Buffer.from(createPublicKey(key).export({ format: 'jwk' }).x, 'base64url');
  const id = createHash('sha256').update(pk).digest().subarray(0, 8);
  const sig = sign(null, message, key);
  const global = sign(null, Buffer.concat([sig, Buffer.from(trusted)]), key);
  return `untrusted comment: signature from the coursive-plugins index key\n${Buffer.concat([Buffer.from('Ed'), id, sig]).toString('base64')}\ntrusted comment: ${trusted}\n${global.toString('base64')}\n`;
}

// verifyMinisign reports whether sigText is a valid signature of message by
// the key in pubText (a minisign public key file).
export function verifyMinisign(message, sigText, pubText) {
  const pub = decode(pubText.trim().split('\n').pop(), 42, 'key');
  const lines = sigText.split('\n');
  const sig = decode(lines[1], 74, 'signature');
  const trusted = (lines[2] ?? '').replace(/^trusted comment: /, '');
  if (!lines[2]?.startsWith('trusted comment: ') || !sig.subarray(2, 10).equals(pub.subarray(2, 10))) return false;
  const key = createPublicKey({ key: { kty: 'OKP', crv: 'Ed25519', x: pub.subarray(10).toString('base64url') }, format: 'jwk' });
  const body = sig.subarray(10);
  return verify(null, message, key, body) && verify(null, Buffer.concat([body, Buffer.from(trusted)]), key, Buffer.from(lines[3] ?? '', 'base64'));
}
