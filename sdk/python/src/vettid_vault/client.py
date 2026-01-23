"""
VettID Service Vault Client for Python.
"""

from datetime import datetime
from typing import Any

import httpx

from .types import (
    VaultConfig,
    APIError,
    AuthResponse,
    AuthzResponse,
    RequestStatus,
    ConnectionContract,
    ContractStatus,
    CallResult,
    CallStatus,
    PaymentResult,
    PaymentStatus,
    SecretResult,
    Money,
)


class VaultError(Exception):
    """Error from the Service Vault API."""

    def __init__(self, error: APIError):
        super().__init__(error.message)
        self.code = error.code
        self.details = error.details


class ServiceVaultClient:
    """
    VettID Service Vault Client.

    Example:
        >>> client = ServiceVaultClient(
        ...     base_url="http://localhost:8080",
        ...     api_key="your-api-key"
        ... )
        >>>
        >>> # Request authentication
        >>> request_id = await client.request_auth(
        ...     user_id="user123",
        ...     purpose="Login to dashboard",
        ...     callback_url="https://myservice.com/webhook"
        ... )
    """

    def __init__(
        self,
        base_url: str,
        api_key: str,
        timeout: float = 30.0,
        retries: int = 3,
        headers: dict[str, str] | None = None,
    ):
        self.config = VaultConfig(
            base_url=base_url.rstrip("/"),
            api_key=api_key,
            timeout=timeout,
            retries=retries,
            headers=headers or {},
        )
        self._client = httpx.AsyncClient(
            base_url=self.config.base_url,
            timeout=self.config.timeout,
            headers={
                "Authorization": f"Bearer {self.config.api_key}",
                "Content-Type": "application/json",
                **self.config.headers,
            },
        )

    async def close(self) -> None:
        """Close the HTTP client."""
        await self._client.aclose()

    async def __aenter__(self) -> "ServiceVaultClient":
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()

    # =========================================================================
    # Authentication
    # =========================================================================

    async def request_auth(
        self,
        user_id: str,
        purpose: str,
        *,
        context: dict[str, Any] | None = None,
        expires_in: int | None = None,
        offline_grace: int | None = None,
        callback_url: str | None = None,
    ) -> str:
        """
        Request authentication from a user.

        Args:
            user_id: The user to authenticate
            purpose: Human-readable purpose for the auth request
            context: Additional context data
            expires_in: Seconds until request expires
            offline_grace: Offline grace period in milliseconds
            callback_url: URL to receive webhook callback

        Returns:
            Request ID (response comes via webhook)
        """
        response = await self._post(
            "/api/v1/auth/request",
            {
                "user_id": user_id,
                "purpose": purpose,
                "context": context,
                "expires_in": expires_in,
                "offline_grace": offline_grace,
                "callback_url": callback_url,
            },
        )
        return response["request_id"]

    async def get_auth_request(self, request_id: str) -> AuthResponse:
        """Get the status of an authentication request."""
        data = await self._get(f"/api/v1/auth/request/{request_id}")
        return AuthResponse(
            request_id=data["request_id"],
            status=RequestStatus(data["status"]),
            user_id=data["user_id"],
            timestamp=datetime.fromisoformat(data["timestamp"]),
            session_key=data.get("session_key"),
        )

    # =========================================================================
    # Authorization
    # =========================================================================

    async def request_authz(
        self,
        user_id: str,
        action: str,
        resource: str,
        *,
        context: dict[str, Any] | None = None,
        expires_in: int | None = None,
        offline_grace: int | None = None,
        callback_url: str | None = None,
    ) -> str:
        """
        Request authorization for an action from a user.

        Args:
            user_id: The user to authorize
            action: Action being requested (e.g., "transfer_funds")
            resource: Resource being accessed (e.g., "account:12345")
            context: Additional context data
            expires_in: Seconds until request expires
            offline_grace: Offline grace period in milliseconds
            callback_url: URL to receive webhook callback

        Returns:
            Request ID
        """
        response = await self._post(
            "/api/v1/authz/request",
            {
                "user_id": user_id,
                "action": action,
                "resource": resource,
                "context": context,
                "expires_in": expires_in,
                "offline_grace": offline_grace,
                "callback_url": callback_url,
            },
        )
        return response["request_id"]

    async def get_authz_request(self, request_id: str) -> AuthzResponse:
        """Get the status of an authorization request."""
        data = await self._get(f"/api/v1/authz/request/{request_id}")
        return AuthzResponse(
            request_id=data["request_id"],
            status=RequestStatus(data["status"]),
            user_id=data["user_id"],
            action=data["action"],
            resource=data["resource"],
            timestamp=datetime.fromisoformat(data["timestamp"]),
            expires_at=(
                datetime.fromisoformat(data["expires_at"])
                if data.get("expires_at")
                else None
            ),
        )

    # =========================================================================
    # Contracts
    # =========================================================================

    async def list_contracts(
        self,
        *,
        status: str | None = None,
        limit: int | None = None,
        offset: int | None = None,
    ) -> list[ConnectionContract]:
        """List all contracts."""
        params: dict[str, Any] = {}
        if status:
            params["status"] = status
        if limit:
            params["limit"] = limit
        if offset:
            params["offset"] = offset

        data = await self._get("/api/v1/contracts", params=params)
        return [self._map_contract(c) for c in data.get("contracts", [])]

    async def get_contract(self, contract_id: str) -> ConnectionContract:
        """Get a specific contract."""
        data = await self._get(f"/api/v1/contracts/{contract_id}")
        return self._map_contract(data)

    async def generate_invite(
        self,
        *,
        offering_id: str | None = None,
        expires_in: int | None = None,
    ) -> dict[str, Any]:
        """Generate a connection invite."""
        data = await self._post(
            "/api/v1/contracts/invite",
            {"offering_id": offering_id, "expires_in": expires_in},
        )
        return {
            "invite_url": data["invite_url"],
            "invite_code": data["invite_code"],
            "expires_at": datetime.fromisoformat(data["expires_at"]),
        }

    async def cancel_contract(
        self, contract_id: str, reason: str | None = None
    ) -> None:
        """Cancel a contract."""
        await self._delete(f"/api/v1/contracts/{contract_id}", {"reason": reason})

    # =========================================================================
    # Calls
    # =========================================================================

    async def initiate_call(
        self,
        user_id: str,
        call_type: str,
        *,
        purpose: str | None = None,
        context: dict[str, Any] | None = None,
        ice_servers: list[dict[str, Any]] | None = None,
        offer: dict[str, Any] | None = None,
        expires_in: int | None = None,
        callback_url: str | None = None,
    ) -> dict[str, Any]:
        """Initiate a call to a user."""
        data = await self._post(
            "/api/v1/call/initiate",
            {
                "user_id": user_id,
                "type": call_type,
                "purpose": purpose,
                "context": context,
                "ice_servers": ice_servers,
                "offer": offer,
                "expires_in": expires_in,
                "callback_url": callback_url,
            },
        )
        return {
            "call_id": data["call_id"],
            "expires_at": datetime.fromisoformat(data["expires_at"]),
        }

    async def get_call_status(self, call_id: str) -> CallResult:
        """Get the status of a call."""
        data = await self._get(f"/api/v1/call/{call_id}")
        return CallResult(
            call_id=data["call_id"],
            request_id=data.get("request_id", ""),
            status=CallStatus(data["status"]),
            answer=data.get("answer"),
            started_at=(
                datetime.fromisoformat(data["started_at"])
                if data.get("started_at")
                else None
            ),
            connected_at=(
                datetime.fromisoformat(data["connected_at"])
                if data.get("connected_at")
                else None
            ),
            ended_at=(
                datetime.fromisoformat(data["ended_at"])
                if data.get("ended_at")
                else None
            ),
            duration=data.get("duration_seconds"),
        )

    async def end_call(self, call_id: str, reason: str | None = None) -> None:
        """End an active call."""
        await self._post(f"/api/v1/call/{call_id}/end", {"reason": reason})

    # =========================================================================
    # Payments
    # =========================================================================

    async def request_payment(
        self,
        user_id: str,
        amount: Money,
        description: str,
        *,
        merchant_info: dict[str, Any] | None = None,
        items: list[dict[str, Any]] | None = None,
        allowed_methods: list[str] | None = None,
        recurring_info: dict[str, Any] | None = None,
        callback_url: str | None = None,
        expires_in: int | None = None,
        metadata: dict[str, Any] | None = None,
    ) -> dict[str, Any]:
        """Request a payment from a user."""
        data = await self._post(
            "/api/v1/payment/request",
            {
                "user_id": user_id,
                "amount": {"amount": amount.amount, "currency": amount.currency},
                "description": description,
                "merchant_info": merchant_info,
                "items": items,
                "allowed_methods": allowed_methods,
                "recurring_info": recurring_info,
                "callback_url": callback_url,
                "expires_in": expires_in,
                "metadata": metadata,
            },
        )
        return {
            "request_id": data["request_id"],
            "expires_at": datetime.fromisoformat(data["expires_at"]),
        }

    async def get_payment_status(self, request_id: str) -> PaymentResult:
        """Get the status of a payment request."""
        data = await self._get(f"/api/v1/payment/request/{request_id}")
        return PaymentResult(
            request_id=data["request_id"],
            status=PaymentStatus(data["status"]),
            payment_id=data.get("payment_id"),
            transaction_ref=data.get("transaction_id"),
            receipt_url=data.get("receipt_url"),
            completed_at=(
                datetime.fromisoformat(data["completed_at"])
                if data.get("completed_at")
                else None
            ),
            failed_at=(
                datetime.fromisoformat(data["failed_at"])
                if data.get("failed_at")
                else None
            ),
            failure_reason=data.get("failure_reason"),
        )

    async def complete_payment(
        self,
        request_id: str,
        transaction_id: str,
        receipt_url: str | None = None,
    ) -> None:
        """Mark a payment as completed."""
        await self._post(
            f"/api/v1/payment/request/{request_id}/complete",
            {"transaction_id": transaction_id, "receipt_url": receipt_url},
        )

    async def fail_payment(self, request_id: str, reason: str) -> None:
        """Mark a payment as failed."""
        await self._post(
            f"/api/v1/payment/request/{request_id}/fail", {"reason": reason}
        )

    async def refund_payment(
        self,
        request_id: str,
        amount: int | None = None,
        reason: str | None = None,
    ) -> None:
        """Refund a payment."""
        await self._post(
            f"/api/v1/payment/request/{request_id}/refund",
            {"amount": amount, "reason": reason},
        )

    # =========================================================================
    # Secrets
    # =========================================================================

    async def store_secret(
        self,
        user_id: str,
        secret_type: str,
        name: str,
        data: bytes,
        *,
        description: str | None = None,
        metadata: dict[str, Any] | None = None,
        callback_url: str | None = None,
        expires_in: int | None = None,
    ) -> dict[str, Any]:
        """Store a secret in the user's vault."""
        import base64

        response = await self._post(
            "/api/v1/secrets/store",
            {
                "user_id": user_id,
                "secret_type": secret_type,
                "name": name,
                "description": description,
                "data": base64.b64encode(data).decode(),
                "metadata": metadata,
                "callback_url": callback_url,
                "expires_in": expires_in,
            },
        )
        return {
            "request_id": response["request_id"],
            "expires_at": datetime.fromisoformat(response["expires_at"]),
        }

    async def retrieve_secret(
        self,
        user_id: str,
        secret_id: str,
        *,
        purpose: str | None = None,
        callback_url: str | None = None,
        expires_in: int | None = None,
    ) -> dict[str, Any]:
        """Retrieve a secret from the user's vault."""
        response = await self._post(
            "/api/v1/secrets/retrieve",
            {
                "user_id": user_id,
                "secret_id": secret_id,
                "purpose": purpose,
                "callback_url": callback_url,
                "expires_in": expires_in,
            },
        )
        return {
            "request_id": response["request_id"],
            "expires_at": datetime.fromisoformat(response["expires_at"]),
        }

    async def delete_secret(
        self,
        user_id: str,
        secret_id: str,
        callback_url: str | None = None,
    ) -> dict[str, Any]:
        """Delete a secret from the user's vault."""
        params = {"user_id": user_id}
        if callback_url:
            params["callback_url"] = callback_url

        response = await self._delete(f"/api/v1/secrets/{secret_id}", params=params)
        return {
            "request_id": response["request_id"],
            "expires_at": datetime.fromisoformat(response["expires_at"]),
        }

    # =========================================================================
    # Health
    # =========================================================================

    async def health(self) -> bool:
        """Check if the vault is healthy."""
        try:
            data = await self._get("/health")
            return data.get("status") == "ok"
        except Exception:
            return False

    # =========================================================================
    # Private HTTP Methods
    # =========================================================================

    async def _get(
        self, path: str, params: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        response = await self._client.get(path, params=params)
        return self._handle_response(response)

    async def _post(
        self, path: str, body: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        response = await self._client.post(path, json=body)
        return self._handle_response(response)

    async def _delete(
        self, path: str, body: dict[str, Any] | None = None, params: dict[str, Any] | None = None
    ) -> dict[str, Any]:
        response = await self._client.request("DELETE", path, json=body, params=params)
        return self._handle_response(response)

    def _handle_response(self, response: httpx.Response) -> dict[str, Any]:
        if response.status_code >= 400:
            try:
                error_data = response.json()
                raise VaultError(
                    APIError(
                        code=error_data.get("code", "internal_error"),
                        message=error_data.get("message", f"HTTP {response.status_code}"),
                        details=error_data.get("details"),
                    )
                )
            except ValueError:
                raise VaultError(
                    APIError(code="internal_error", message=f"HTTP {response.status_code}")
                )

        try:
            return response.json()
        except ValueError:
            return {}

    def _map_contract(self, data: dict[str, Any]) -> ConnectionContract:
        return ConnectionContract(
            contract_id=data["contract_id"],
            user_id=data["user_id"],
            service_id=data["service_id"],
            offering_id=data["offering_id"],
            status=ContractStatus(data["status"]),
            created_at=datetime.fromisoformat(data["created_at"]),
            activated_at=(
                datetime.fromisoformat(data["activated_at"])
                if data.get("activated_at")
                else None
            ),
            cancelled_at=(
                datetime.fromisoformat(data["cancelled_at"])
                if data.get("cancelled_at")
                else None
            ),
        )
