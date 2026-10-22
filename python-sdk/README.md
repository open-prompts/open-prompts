# OpenPrompts Python SDK

The official Python server SDK for [OpenPrompts](https://github.com/open-prompts/open-prompts).

## Installation

```bash
pip install openprompts
```

## Quick Start

```python
import openprompts

# Initialize the client
op = openprompts.init("https://your-openprompts-server.com", api_key="YOUR_API_KEY", ssl_verify=True)

# Fetch a prompt by template ID and tag (alias)
try:
    prompt = op.get_prompt(template_id="your-template-id", prompt_tag="latest")
    print("Prompt content:", prompt.get("content"))
except openprompts.OpenPromptsError as e:
    print(f"Error fetching prompt: {e}")
```

## Comments

```python
# Post a top-level comment on a template
comment = op.create_comment(template_id="your-template-id", content="Nice prompt!")

# Reply to a top-level comment (single-level threading)
reply = op.create_comment(
    template_id="your-template-id",
    content="Thanks for the reply!",
    parent_comment_id=comment["id"],
)

# List all replies to a top-level comment, oldest-first (≤ 50)
replies = op.list_comment_replies(comment_id=comment["id"])
for r in replies.get("replies", []):
    print(r["author"]["display_name"], ":", r["content"])
```
