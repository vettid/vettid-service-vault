/**
 * Cryptographic utilities for VettID Service Vault
 *
 * Implements Ed25519 signing, X25519 encryption, and service identity generation.
 */

import nacl from 'tweetnacl';
import { encodeBase64, decodeBase64 } from 'tweetnacl-util';
import bs58 from 'bs58';
import { createHash } from 'crypto';
import { ServiceIdentity, ServiceKeyPair, ServiceType } from './types';

/**
 * Generate a new service identity with cryptographic keys.
 *
 * @param serviceName - Human-readable name for the service
 * @param serviceType - Category of service
 * @returns Service identity and keypair
 *
 * @example
 * ```typescript
 * const { identity, keyPair } = generateServiceIdentity('My Service', 'generic');
 * console.log('Service ID:', identity.serviceId);
 * // Store keyPair securely!
 * ```
 */
export function generateServiceIdentity(
  serviceName: string,
  serviceType: ServiceType = 'generic'
): { identity: ServiceIdentity; keyPair: ServiceKeyPair } {
  // Generate Ed25519 signing keypair
  const signingKeyPair = nacl.sign.keyPair();

  // Generate X25519 encryption keypair
  const encryptionKeyPair = nacl.box.keyPair();

  // Derive service ID: base58(sha256(signing_public_key)[0:20])
  const hash = createHash('sha256').update(signingKeyPair.publicKey).digest();
  const serviceId = bs58.encode(hash.slice(0, 20));

  const keyPair: ServiceKeyPair = {
    signingPrivateKey: signingKeyPair.secretKey,
    signingPublicKey: signingKeyPair.publicKey,
    encryptionPrivateKey: encryptionKeyPair.secretKey,
    encryptionPublicKey: encryptionKeyPair.publicKey,
  };

  const identity: ServiceIdentity = {
    serviceId,
    serviceName,
    serviceType,
    signingPublicKey: encodeBase64(signingKeyPair.publicKey),
    encryptionPublicKey: encodeBase64(encryptionKeyPair.publicKey),
  };

  return { identity, keyPair };
}

/**
 * Derive a service ID from a public key.
 *
 * @param publicKey - Ed25519 public key (Uint8Array or base64 string)
 * @returns Service ID as base58 string
 */
export function deriveServiceId(publicKey: Uint8Array | string): string {
  const keyBytes = typeof publicKey === 'string' ? decodeBase64(publicKey) : publicKey;
  const hash = createHash('sha256').update(keyBytes).digest();
  return bs58.encode(hash.slice(0, 20));
}

/**
 * Sign data with Ed25519.
 *
 * @param data - Data to sign
 * @param privateKey - Ed25519 private key (64 bytes)
 * @returns Signature as Uint8Array
 */
export function sign(data: Uint8Array, privateKey: Uint8Array): Uint8Array {
  return nacl.sign.detached(data, privateKey);
}

/**
 * Verify an Ed25519 signature.
 *
 * @param data - Original data
 * @param signature - Signature to verify
 * @param publicKey - Ed25519 public key
 * @returns True if signature is valid
 */
export function verify(
  data: Uint8Array,
  signature: Uint8Array,
  publicKey: Uint8Array
): boolean {
  return nacl.sign.detached.verify(data, signature, publicKey);
}

/**
 * Encrypt a message for a recipient using X25519 + XSalsa20-Poly1305.
 *
 * Note: This uses NaCl's box which is XSalsa20-Poly1305, not XChaCha20-Poly1305.
 * For production, consider using a library that supports XChaCha20-Poly1305.
 *
 * @param message - Message to encrypt
 * @param recipientPublicKey - Recipient's X25519 public key
 * @param senderPrivateKey - Sender's X25519 private key (optional, generates ephemeral if not provided)
 * @returns Encrypted message with nonce and ephemeral public key
 */
export function encrypt(
  message: Uint8Array,
  recipientPublicKey: Uint8Array,
  senderPrivateKey?: Uint8Array
): { ciphertext: Uint8Array; nonce: Uint8Array; ephemeralPublicKey?: Uint8Array } {
  const nonce = nacl.randomBytes(nacl.box.nonceLength);

  if (senderPrivateKey) {
    const ciphertext = nacl.box(message, nonce, recipientPublicKey, senderPrivateKey);
    return { ciphertext, nonce };
  }

  // Use ephemeral keypair for one-way encryption
  const ephemeral = nacl.box.keyPair();
  const ciphertext = nacl.box(message, nonce, recipientPublicKey, ephemeral.secretKey);
  return { ciphertext, nonce, ephemeralPublicKey: ephemeral.publicKey };
}

/**
 * Decrypt a message using X25519 + XSalsa20-Poly1305.
 *
 * @param ciphertext - Encrypted message
 * @param nonce - Nonce used for encryption
 * @param senderPublicKey - Sender's X25519 public key (or ephemeral public key)
 * @param recipientPrivateKey - Recipient's X25519 private key
 * @returns Decrypted message or null if decryption fails
 */
export function decrypt(
  ciphertext: Uint8Array,
  nonce: Uint8Array,
  senderPublicKey: Uint8Array,
  recipientPrivateKey: Uint8Array
): Uint8Array | null {
  return nacl.box.open(ciphertext, nonce, senderPublicKey, recipientPrivateKey);
}

/**
 * Generate a random nonce.
 *
 * @param length - Nonce length in bytes (default: 24 for NaCl box)
 * @returns Random nonce
 */
export function generateNonce(length: number = 24): Uint8Array {
  return nacl.randomBytes(length);
}

/**
 * Generate random bytes.
 *
 * @param length - Number of bytes
 * @returns Random bytes
 */
export function randomBytes(length: number): Uint8Array {
  return nacl.randomBytes(length);
}

/**
 * Encode bytes to base64.
 */
export { encodeBase64, decodeBase64 };

/**
 * Encode bytes to base58.
 */
export function encodeBase58(bytes: Uint8Array): string {
  return bs58.encode(bytes);
}

/**
 * Decode base58 to bytes.
 */
export function decodeBase58(str: string): Uint8Array {
  return bs58.decode(str);
}

/**
 * Canonicalize an object for signing.
 * Produces deterministic JSON output.
 *
 * @param obj - Object to canonicalize
 * @returns Canonical JSON bytes
 */
export function canonicalize(obj: unknown): Uint8Array {
  const canonical = JSON.stringify(obj, Object.keys(obj as object).sort());
  return new TextEncoder().encode(canonical);
}

/**
 * Create a signature over a canonical JSON object.
 *
 * @param obj - Object to sign
 * @param privateKey - Ed25519 private key
 * @returns Base64-encoded signature
 */
export function signObject(obj: unknown, privateKey: Uint8Array): string {
  const data = canonicalize(obj);
  const signature = sign(data, privateKey);
  return encodeBase64(signature);
}

/**
 * Verify a signature over a canonical JSON object.
 *
 * @param obj - Object that was signed
 * @param signature - Base64-encoded signature
 * @param publicKey - Ed25519 public key
 * @returns True if signature is valid
 */
export function verifyObject(
  obj: unknown,
  signature: string,
  publicKey: Uint8Array
): boolean {
  const data = canonicalize(obj);
  const sig = decodeBase64(signature);
  return verify(data, sig, publicKey);
}
