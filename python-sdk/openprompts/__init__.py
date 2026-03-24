from .client import OpenPromptsClient
from .exceptions import OpenPromptsError, APIError, AuthError

def init(base_url: str, api_key: str = None, ssl_verify: bool = True) -> OpenPromptsClient:
    """
    Initialize and return an OpenPrompts client instance.
    
    :param base_url: The base URL of the OpenPrompts server.
    :param api_key: Your API key for authentication.
    :param ssl_verify: Whether to verify SSL certificates. Defaults to True.
    :return: An initialized OpenPromptsClient.
    """
    return OpenPromptsClient(base_url=base_url, api_key=api_key, ssl_verify=ssl_verify)

__all__ = [
    "init",
    "OpenPromptsClient",
    "OpenPromptsError",
    "APIError",
    "AuthError",
]
