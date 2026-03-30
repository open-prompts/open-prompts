import requests
from typing import Optional, Dict, Any

from .exceptions import APIError, AuthError

class OpenPromptsClient:
    def __init__(self, base_url: str, api_key: Optional[str] = None, ssl_verify: bool = True):
        """
        Initialize the OpenPrompts client.
        
        :param base_url: The base URL of the OpenPrompts server.
        :param api_key: Your API key for authentication.
        :param ssl_verify: Whether to verify SSL certificates. Defaults to True.
        """
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self.ssl_verify = ssl_verify
        self.session = requests.Session()
        self.session.verify = self.ssl_verify
        
        if self.api_key:
            self.session.headers.update({
                "Authorization": f"Bearer {self.api_key}"
            })
            
    def _handle_response(self, response: requests.Response) -> Dict[str, Any]:
        if response.ok:
            try:
                return response.json()
            except ValueError:
                return {"message": response.text}
                
        if response.status_code in (401, 403):
            raise AuthError(f"Authentication failed: {response.text}")
            
        raise APIError(
            message=f"API Error ({response.status_code}): {response.text}",
            status_code=response.status_code,
            response=response
        )

    def get_prompt(self, template_id: str, prompt_tag: str = "latest", variables: Optional[Dict[str, Any]] = None) -> str:
        """
        Get a fully rendered prompt string by its template ID and tag (alias).
        This also attempts to generate and store the prompt instance on the server.
        
        :param template_id: The ID of the template.
        :param prompt_tag: The alias/tag of the template version (e.g. 'latest').
        :param variables: Dictionary of variables to replace in the prompt template (e.g., {'topic': 'AI'}).
        :return: A string containing the final generated prompt.
        """
        # First get the template version using the alias
        url = f"{self.base_url}/api/v1/templates/{template_id}/aliases/{prompt_tag}"
        resp = self.session.get(url)
        tv_data = self._handle_response(resp)
        version_id = tv_data.get("id")

        if not version_id:
            return ""

        # Generate prompt instance on backend and retrieve the rendered content
        gen_url = f"{self.base_url}/api/v1/prompts"
        payload = {
            "template_id": template_id,
            "version_id": version_id,
        }
        if variables:
            payload["variables"] = variables

        try:
            gen_resp = self.session.post(gen_url, json=payload)
            prompt_data = self._handle_response(gen_resp)
            if "prompt" in prompt_data:
                return prompt_data["prompt"].get("content", "")
            return prompt_data.get("content", "")
        except APIError as e:
            # Fallback to local rendering if generating prompt fails/unsupported
            content = tv_data.get("content", "")
            if variables:
                for k, v in variables.items():
                    content = content.replace(f"${{{k}}}", str(v))
            return content
        
    def get_template_version(self, template_id: str, prompt_tag: str = "latest") -> Dict[str, Any]:
        """
        Get raw template version metadata by its template ID and tag (alias).
        
        :param template_id: The ID of the template.
        :param prompt_tag: The alias/tag of the template version.
        :return: A dictionary containing the template's metadata and unrendered content.
        """
        url = f"{self.base_url}/api/v1/templates/{template_id}/aliases/{prompt_tag}"
        resp = self.session.get(url)
        return self._handle_response(resp)

    def list_templates(self, page_size: int = 10, page_token: str = "", **kwargs) -> Dict[str, Any]:
        """
        List templates available in the system.
        
        :param page_size: Number of templates to return per page.
        :param page_token: Pagination token for the next page.
        :param kwargs: Additional query parameters (e.g. 'category', 'language', 'tags', 'visibility').
        :return: A dictionary of templates.
        """
        url = f"{self.base_url}/api/v1/templates"
        params = {"page_size": page_size}
        if page_token:
            params["page_token"] = page_token
        params.update(kwargs)
        
        resp = self.session.get(url, params=params)
        return self._handle_response(resp)
        
    def get_template(self, template_id: str) -> Dict[str, Any]:
        """
        Get detailed information about a template by its ID.
        
        :param template_id: The ID of the template.
        :return: A dictionary with the template details.
        """
        url = f"{self.base_url}/api/v1/templates/{template_id}"
        resp = self.session.get(url)
        return self._handle_response(resp)
