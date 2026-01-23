"""
Cryptographic utilities for VettID Service Vault.
"""

import hashlib
import json
from typing import Any

import base58
import nacl.public
import nacl.signing
import nacl.utils

from .types import ServiceIdentity, ServiceKeyPair, ServiceType


def generate_service_identity(
    service_name: str,
    service_type: ServiceType = ServiceType.GENERIC,
) -> tuple[ServiceIdentity, ServiceKeyPair]:
    """
    Generate a new service identity with cryptographic keys.

    Args:
        service_name: Human-readable name for the service
        service_type: Category of service

    Returns:
        Tuple of (identity, keypair)

    Example:
        >>> identity, keypair = generate_service_identity("My Service", ServiceType.GENERIC)
        >>> print(f"Service ID: {identity.service_id}")
    """
    # Generate Ed25519 signing keypair
    signing_key = nacl.signing.SigningKey.generate()
    verify_key = signing_key.verify_key

    # Generate X25519 encryption keypair
    encryption_private_key = nacl.public.PrivateKey.generate()
    encryption_public_key = encryption_private_key.public_key

    # Derive service ID: base58(sha256(signing_public_key)[0:20])
    hash_bytes = hashlib.sha256(bytes(verify_key)).digest()
    service_id = base58.b58encode(hash_bytes[:20]).decode()

    keypair = ServiceKeyPair(
        signing_private_key=bytes(signing_key),
        signing_public_key=bytes(verify_key),
        encryption_private_key=bytes(encryption_private_key),
        encryption_public_key=bytes(encryption_public_key),
    )

    import base64

    identity = ServiceIdentity(
        service_id=service_id,
        service_name=service_name,
        service_type=service_type,
        signing_public_key=base64.b64encode(bytes(verify_key)).decode(),
        encryption_public_key=base64.b64encode(bytes(encryption_public_key)).decode(),
    )

    return identity, keypair


def derive_service_id(public_key: bytes | str) -> str:
    """
    Derive a service ID from a public key.

    Args:
        public_key: Ed25519 public key (bytes or base64 string)

    Returns:
        Service ID as base58 string
    """
    import base64

    if isinstance(public_key, str):
        key_bytes = base64.b64decode(public_key)
    else:
        key_bytes = public_key

    hash_bytes = hashlib.sha256(key_bytes).digest()
    return base58.b58encode(hash_bytes[:20]).decode()


def sign(data: bytes, private_key: bytes) -> bytes:
    """
    Sign data with Ed25519.

    Args:
        data: Data to sign
        private_key: Ed25519 private key (64 bytes seed + public key)

    Returns:
        64-byte signature
    """
    signing_key = nacl.signing.SigningKey(private_key[:32])
    signed = signing_key.sign(data)
    return signed.signature


def verify(data: bytes, signature: bytes, public_key: bytes) -> bool:
    """
    Verify an Ed25519 signature.

    Args:
        data: Original data
        signature: 64-byte signature
        public_key: Ed25519 public key

    Returns:
        True if signature is valid
    """
    verify_key = nacl.signing.VerifyKey(public_key)
    try:
        verify_key.verify(data, signature)
        return True
    except nacl.exceptions.BadSignature:
        return False


def encrypt(
    message: bytes,
    recipient_public_key: bytes,
    sender_private_key: bytes | None = None,
) -> tuple[bytes, bytes, bytes | None]:
    """
    Encrypt a message for a recipient using X25519 + XSalsa20-Poly1305.

    Args:
        message: Message to encrypt
        recipient_public_key: Recipient's X25519 public key
        sender_private_key: Sender's X25519 private key (optional)

    Returns:
        Tuple of (ciphertext, nonce, ephemeral_public_key)
        ephemeral_public_key is None if sender_private_key was provided
    """
    recipient_pk = nacl.public.PublicKey(recipient_public_key)

    if sender_private_key:
        sender_sk = nacl.public.PrivateKey(sender_private_key)
        box = nacl.public.Box(sender_sk, recipient_pk)
        nonce = nacl.utils.random(nacl.public.Box.NONCE_SIZE)
        ciphertext = box.encrypt(message, nonce).ciphertext
        return ciphertext, nonce, None

    # Use ephemeral keypair
    ephemeral_sk = nacl.public.PrivateKey.generate()
    ephemeral_pk = ephemeral_sk.public_key
    box = nacl.public.Box(ephemeral_sk, recipient_pk)
    nonce = nacl.utils.random(nacl.public.Box.NONCE_SIZE)
    ciphertext = box.encrypt(message, nonce).ciphertext
    return ciphertext, nonce, bytes(ephemeral_pk)


def decrypt(
    ciphertext: bytes,
    nonce: bytes,
    sender_public_key: bytes,
    recipient_private_key: bytes,
) -> bytes | None:
    """
    Decrypt a message using X25519 + XSalsa20-Poly1305.

    Args:
        ciphertext: Encrypted message
        nonce: Nonce used for encryption
        sender_public_key: Sender's X25519 public key
        recipient_private_key: Recipient's X25519 private key

    Returns:
        Decrypted message or None if decryption fails
    """
    sender_pk = nacl.public.PublicKey(sender_public_key)
    recipient_sk = nacl.public.PrivateKey(recipient_private_key)
    box = nacl.public.Box(recipient_sk, sender_pk)
    try:
        return box.decrypt(ciphertext, nonce)
    except nacl.exceptions.CryptoError:
        return None


def canonicalize(obj: Any) -> bytes:
    """
    Canonicalize an object for signing.

    Args:
        obj: Object to canonicalize

    Returns:
        Canonical JSON bytes
    """
    return json.dumps(obj, sort_keys=True, separators=(",", ":")).encode()


def sign_object(obj: Any, private_key: bytes) -> str:
    """
    Sign a canonical JSON object.

    Args:
        obj: Object to sign
        private_key: Ed25519 private key

    Returns:
        Base64-encoded signature
    """
    import base64

    data = canonicalize(obj)
    signature = sign(data, private_key)
    return base64.b64encode(signature).decode()


def verify_object(obj: Any, signature: str, public_key: bytes) -> bool:
    """
    Verify a signature over a canonical JSON object.

    Args:
        obj: Object that was signed
        signature: Base64-encoded signature
        public_key: Ed25519 public key

    Returns:
        True if signature is valid
    """
    import base64

    data = canonicalize(obj)
    sig = base64.b64decode(signature)
    return verify(data, sig, public_key)
