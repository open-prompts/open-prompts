class OpenPromptsError(Exception):
    """Base class for exceptions in this module."""
    pass

class AuthError(OpenPromptsError):
    """Exception raised for authentication errors."""
    pass

class APIError(OpenPromptsError):
    """Exception raised for API-related errors."""
    def __init__(self, message, status_code=None, response=None):
        super().__init__(message)
        self.status_code = status_code
        self.response = response
