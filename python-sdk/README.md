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
