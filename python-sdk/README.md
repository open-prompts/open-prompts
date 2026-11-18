# Open Prompts Python SDK

Python client for the Open Prompts REST API.

The SDK currently covers prompt retrieval, template and version lookup, template listing, comment creation, and comment-reply listing. It does not wrap the full REST API.

## Requirements

- Python 3.7 or later
- `requests` 2.20 or later (installed as a dependency)

## Installation

```bash
pip install openprompts
```

## Initialize the client

```python
import os
import openprompts

client = openprompts.init(
    "http://localhost:3000",
    api_key=os.environ["OPEN_PROMPTS_API_KEY"],
    ssl_verify=True,
)
```

`base_url` is the server root. Do not append `/api/v1`; the SDK adds it to each request. For a local backend, use `http://localhost:8080`. For the Docker frontend, use `http://localhost:3000`.

`api_key` is optional at initialization, but authenticated methods require a valid key. `ssl_verify` defaults to `True`; leave it enabled unless you are testing a server with a self-signed certificate.

## Get a rendered prompt

```python
rendered = client.get_prompt(
    template_id="TEMPLATE_ID",
    prompt_tag="latest",
    variables={"article": "Open Prompts stores versioned prompt content."},
)
print(rendered)
```

`get_prompt` returns the rendered content as a string. On the normal path it performs two requests:

1. `GET /api/v1/templates/{template_id}/aliases/{prompt_tag}` to resolve the version.
2. `POST /api/v1/prompts` to render and save the prompt.

The second request requires authentication, so `get_prompt` needs a valid API key. If that POST request fails with `APIError`, the SDK falls back to local `${key}` substitution using the version content. `AuthError` is not caught and is raised to the caller.

The alias lookup is a public read. As a result, this method does not resolve private templates.

If the alias response has no version `id`, `get_prompt` returns an empty string.

## Methods

### `get_prompt(template_id, prompt_tag="latest", variables=None) -> str`

Resolves the alias, renders the prompt, and stores it on the server. Requires a valid API key.

- `template_id` (str): template identifier.
- `prompt_tag` (str): alias name, default `"latest"`.
- `variables` (dict, optional): values for `${name}` placeholders. Missing variables remain unchanged.

Returns the rendered prompt as a string.

### `get_template_version(template_id, prompt_tag="latest") -> dict`

Returns the raw version metadata and unrendered content from the alias lookup. This is a public read and does not resolve private templates.

### `get_template(template_id) -> dict`

Returns template detail. When the client has a valid API key, the key identifies the caller, so the owner can read their own private template.

### `list_templates(page_size=10, page_token="", **kwargs) -> dict`

Returns the template list response. Additional keyword arguments are passed as query parameters.

Use the REST query names and values. For example, `visibility` is accepted as the enum string `VISIBILITY_PUBLIC` or `VISIBILITY_PRIVATE`, and repeated tags can be passed as a list:

```python
result = client.list_templates(
    page_size=20,
    category="writing",
    language="en",
    tags=["summary", "writing"],
    sort="newest",
)
```

Supported query parameters include `category`, `language`, `owner_id`, `tags`, `visibility`, `my_likes`, `my_favorites`, `keyword`, `q`, `sort`, and `exclude_warned`. `sort` accepts `newest`, `most_liked`, `most_favorited`, `trending`, `top_rated`, and `updated`. `visibility` must be the enum string `VISIBILITY_PUBLIC` or `VISIBILITY_PRIVATE`; lowercase `public` and `private` are ignored by the REST list handler. Anonymous requests cannot list private templates, so `VISIBILITY_PRIVATE` has no effect without a valid API key. The boolean flags `my_likes` and `my_favorites` require an authenticated client; without an API key they are ignored. Pass all three boolean flags as the lowercase string `"true"`; Python `True` is sent as `True` and is ignored by the handler. List values such as `tags` are sent as repeated query parameters.

### `create_comment(template_id, content, parent_comment_id=None) -> dict`

Creates a comment. Requires a valid API key.

- `content` must be 1-2,000 characters after trimming.
- `parent_comment_id` is optional. When set, it must identify a top-level comment; replies to replies are rejected.
- Returns the created comment.

```python
comment = client.create_comment(
    template_id="TEMPLATE_ID",
    content="A useful template.",
)

reply = client.create_comment(
    template_id="TEMPLATE_ID",
    content="Thanks for the feedback.",
    parent_comment_id=comment["id"],
)
```

### `list_comment_replies(comment_id) -> dict`

Returns replies to a top-level comment, oldest first, at most 50.

```python
replies = client.list_comment_replies(comment_id=comment["id"])
for reply in replies.get("replies", []):
    print(reply["author"]["display_name"], ":", reply["content"])
```

## Errors

- `OpenPromptsError` is the base class for SDK errors.
- `AuthError` is raised for HTTP 401 and 403 responses.
- `APIError` is raised for other non-success HTTP responses and exposes `status_code` and `response`.
- Network and transport errors raised by `requests` are not wrapped in `OpenPromptsError`.

```python
try:
    rendered = client.get_prompt("TEMPLATE_ID", variables={"topic": "example"})
except openprompts.AuthError as exc:
    print("Check the API key:", exc)
except openprompts.APIError as exc:
    print("Request failed:", exc.status_code)
except openprompts.OpenPromptsError as exc:
    print("SDK error:", exc)
```

## Notes

- The SDK is synchronous and uses `requests.Session`.
- The client does not set a request timeout or implement retries.
- The alias lookup used by `get_prompt` is public, so the method cannot resolve private templates.
- `get_prompt` and `create_comment` require a valid API key. `get_template`, `list_templates`, and `list_comment_replies` can be used anonymously for public resources; a valid key is needed to access the caller's private resources where the endpoint supports it. `get_template_version` reads the alias endpoint, which does not use the caller identity, so it cannot resolve private templates even when a key is configured.
