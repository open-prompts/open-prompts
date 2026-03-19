import subprocess
import time
import requests
import sys
import os
import json
import atexit

BASE_URL = "http://localhost:8080/api/v1/templates"
PROMPT_URL = "http://localhost:8080/api/v1/prompts"

# Resource Tracking
CREATED_TEMPLATES = [] # List of {'id': uuid, 'owner_id': string}
CREATED_PROMPTS = []   # List of {'id': uuid, 'owner_id': string}

def cleanup():
    print("\n--- Starting Clean Up ---")

    # Cleanup Prompts
    for p in reversed(CREATED_PROMPTS):
        try:
            token = get_auth_token(p['owner_id'])
            headers = {"Authorization": f"Bearer {token}"}
            url = f"{PROMPT_URL}/{p['id']}?owner_id={p['owner_id']}"
            resp = requests.delete(url, headers=headers)
            if resp.status_code in [200, 404]:
                print(f"Cleaned prompt {p['id']}")
            else:
                print(f"Failed to clean prompt {p['id']}: {resp.status_code}")
        except Exception as e:
            print(f"Error cleaning prompt {p['id']}: {e}")

    # Cleanup Templates
    for t in reversed(CREATED_TEMPLATES):
        try:
            token = get_auth_token(t['owner_id'])
            headers = {"Authorization": f"Bearer {token}"}
            url = f"{BASE_URL}/{t['id']}?owner_id={t['owner_id']}"
            resp = requests.delete(url, headers=headers)
            if resp.status_code in [200, 404]:
                print(f"Cleaned template {t['id']}")
            else:
                print(f"Failed to clean template {t['id']}: {resp.status_code}")
        except Exception as e:
            print(f"Error cleaning template {t['id']}: {e}")

    print("--- Clean Up Complete ---")

atexit.register(cleanup)

def run_command(command):
    print(f"Executing: {command}")
    process = subprocess.Popen(command, shell=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    stdout, stderr = process.communicate()
    if process.returncode != 0:
        print(f"Error executing command: {command}")
        print(stderr.decode())
        return False
    return True

def wait_for_service(url, timeout=60):
    start_time = time.time()
    while time.time() - start_time < timeout:
        try:
            response = requests.get(url)
            if response.status_code == 200:
                return True
        except requests.exceptions.ConnectionError:
            pass
        time.sleep(1)
    return False


def get_auth_token(user_id):
    email = f"{user_id}@example.com"
    password = "password123"

    # Try login first
    resp = requests.post("http://localhost:8080/api/v1/login", json={
        "email": email,
        "password": password
    })

    if resp.status_code == 200:
        return resp.json().get("token")

    # Send Verification Code
    requests.post("http://localhost:8080/api/v1/verification-code", json={
        "email": email
    })

    # Register if login failed
    resp = requests.post("http://localhost:8080/api/v1/register", json={
        "id": user_id,
        "email": email,
        "password": password,
        "display_name": "Test User",
        "verification_code": "123456"
    })
    return resp.json().get("token")

def test_profile_update():
    print("--- Starting Profile Update Test ---")
    user_id = f"user_update_{int(time.time())}"
    token = get_auth_token(user_id)
    headers = {"Authorization": f"Bearer {token}"}

    update_url = "http://localhost:8080/api/v1/profile"

    # Update Profile
    update_data = {
        "display_name": "Updated Name",
        "avatar": "data:image/png;base64,fake"
    }
    resp = requests.put(update_url, headers=headers, json=update_data)
    if resp.status_code != 200:
        print(f"FAILED: Profile update failed with {resp.status_code}: {resp.text}")
        sys.exit(1)

    data = resp.json()
    if data['display_name'] != "Updated Name":
        print(f"FAILED: Display name mismatch. Expected 'Updated Name', got {data['display_name']}")
        sys.exit(1)
    if data['avatar'] != "data:image/png;base64,fake":
        print(f"FAILED: Avatar mismatch.")
        sys.exit(1)

    print("PASSED: Profile Update Test")

def test_lifecycle():
    print("--- Starting Lifecycle Test ---")

    owner_id = f"test_user_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}

    # 1. Create Template
    print("1. Creating Template...")
    create_payload = {
        "owner_id": owner_id,
        "title": "My First Template",
        "description": "A test template",
        "visibility": "VISIBILITY_PUBLIC",
        "type": "TEMPLATE_TYPE_USER",
        "tags": ["test", "demo"],
        "category": "general"
    }
    resp = requests.post(BASE_URL, json=create_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create failed: {resp.status_code} - {resp.text}")
        return False

    created_template = resp.json().get("template")
    if not created_template:
        print("Create response missing template")
        return False
    template_id = created_template.get("id")
    print(f"Created Template ID: {template_id}")
    CREATED_TEMPLATES.append({"id": template_id, "owner_id": owner_id})

    # 2. Get Template
    print("2. Getting Template...")
    resp = requests.get(f"{BASE_URL}/{template_id}")
    if resp.status_code != 200:
        print(f"Get failed: {resp.status_code} - {resp.text}")
        return False
    if resp.json().get("template", {}).get("title") != "My First Template":
        print("Get returned incorrect title")
        return False

    # 3. Updating Template (Change to Private)
    print("3. Updating Template...")
    update_payload = {
        "owner_id": owner_id,
        "template_id": template_id,

        "owner_id": owner_id,
        "title": "Updated Template Title",
        "description": "Updated description",
        # Keep PUBLIC so it can be found in the next step (Guest List)
        # OR Update logic to search for it using Auth if we test Private
        "visibility": "VISIBILITY_PUBLIC",
        "tags": ["updated"],
        "category": "updated_cat"
    }
    resp = requests.put(f"{BASE_URL}/{template_id}", json=update_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Update failed: {resp.status_code} - {resp.text}")
        return False
    if resp.json().get("template", {}).get("title") != "Updated Template Title":
        print("Update returned incorrect title")
        return False

    # 4. List Templates
    print("4. Listing Templates...")
    resp = requests.get(BASE_URL)
    if resp.status_code != 200:
        print(f"List failed: {resp.status_code} - {resp.text}")
        return False
    templates = resp.json().get("templates", [])
    found = False
    for t in templates:
        if t.get("id") == template_id:
            found = True
            if t.get("title") != "Updated Template Title":
                print("List returned incorrect title for updated template")
                return False
            break
    if not found:
        print("List did not find the updated template")
        return False

    # 5. Delete Template
    print("5. Deleting Template...")
    resp = requests.delete(f"{BASE_URL}/{template_id}?owner_id={owner_id}", headers=headers)
    if resp.status_code != 200:
        print(f"Delete failed: {resp.status_code} - {resp.text}")
        return False

    # 6. Verify Deletion
    print("6. Verifying Deletion...")
    resp = requests.get(f"{BASE_URL}/{template_id}")
    # Expecting 404
    if resp.status_code != 404:
        print(f"Get after delete should fail with 404, but got {resp.status_code}")
        return False

    print("--- Lifecycle Test Passed ---")
    return True


def test_auth():
    print("--- Starting Auth Test ---")
    REGISTER_URL = "http://localhost:8080/api/v1/register"
    LOGIN_URL = "http://localhost:8080/api/v1/login"

    ts = int(time.time())
    user_id = f"fvt_user_{ts}"
    email = f"fvt_user_{ts}@example.com"
    password = "password123"

    # 1. Register
    print("1. Registering User...")
    # Send verification code
    requests.post("http://localhost:8080/api/v1/verification-code", json={"email": email})

    register_payload = {
        "id": user_id,
        "email": email,
        "password": password,
        "display_name": "FVT User",
        "verification_code": "123456"
    }
    resp = requests.post(REGISTER_URL, json=register_payload)
    if resp.status_code != 200:
        print(f"Register failed: {resp.status_code} - {resp.text}")
        return False

    data = resp.json()
    if data.get("id") != user_id:
        print(f"Register returned wrong ID: {data.get('id')}")
        return False
    if not data.get("token"):
        print("Register response missing token")
        return False

    print("User registered successfully.")

    # 2. Login
    print("2. Logging in...")
    login_payload = {
        "email": email,
        "password": password
    }
    resp = requests.post(LOGIN_URL, json=login_payload)
    if resp.status_code != 200:
        print(f"Login failed: {resp.status_code} - {resp.text}")
        return False

    data = resp.json()
    if data.get("id") != user_id:
        print(f"Login returned wrong ID: {data.get('id')}")
        return False
    if not data.get("token"):
        print("Login response missing token")
        return False

    print("User logged in successfully.")

    # 3. Duplicate Register
    print("3. Testing Duplicate Register...")
    # Send verification code again because the previous one was consumed
    requests.post("http://localhost:8080/api/v1/verification-code", json={"email": email})

    resp = requests.post(REGISTER_URL, json=register_payload)
    if resp.status_code != 409:
        print(f"Duplicate register should fail with 409, but got {resp.status_code}")
        return False
    print("Duplicate register failed as expected.")

    # 4. Invalid Login
    print("4. Testing Invalid Login...")
    invalid_payload = {
        "email": email,
        "password": "wrongpassword"
    }
    resp = requests.post(LOGIN_URL, json=invalid_payload)
    if resp.status_code != 401:
        print(f"Invalid login should fail with 401, but got {resp.status_code}")
        return False
    print("Invalid login failed as expected.")

    print("--- Auth Test Passed ---")
    return True

def test_prompt_lifecycle():
    print("--- Starting Prompt Lifecycle Test ---")

    # 1. Register & Login User 1
    user1_id = f"test_user1_{int(time.time())}"
    email = f"test_user1_{int(time.time())}@example.com"
    password = "password"

    # Send code
    requests.post("http://localhost:8080/api/v1/verification-code", json={"email": email})

    register_payload = {
        "id": user1_id,
        "email": email,
        "password": password,
        "display_name": "User 1",
        "verification_code": "123456"
    }
    resp = requests.post("http://localhost:8080/api/v1/register", json=register_payload)
    if resp.status_code != 200:
        print(f"Register failed: {resp.status_code} - {resp.text}")
        return False
    token = resp.json().get("token")
    headers = {"Authorization": f"Bearer {token}"}

    # 2. Create Template (User 1)
    tpl_payload = {
        "owner_id": user1_id,
        "title": "My Template",
        "description": "Desc",
        "visibility": 1, # Private
        "type": 2, # User
        "tags": ["test"],
        "category": "test",
        "content": "Hello {{name}}"
    }
    resp = requests.post("http://localhost:8080/api/v1/templates", json=tpl_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create Template failed: {resp.status_code} - {resp.text}")
        return False
    tpl_data = resp.json()
    template_id = tpl_data.get("template", {}).get("id")
    version_id = tpl_data.get("version", {}).get("id")

    print(f"Created Template: {template_id}")
    CREATED_TEMPLATES.append({"id": template_id, "owner_id": user1_id})

    # 3. Create Prompt (User 1)
    prompt_payload = {
        "template_id": template_id,
        "version_id": version_id,
        "owner_id": user1_id,
        "variables": ["World"]
    }
    resp = requests.post("http://localhost:8080/api/v1/prompts", json=prompt_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create Prompt failed: {resp.status_code} - {resp.text}")
        return False
    prompt_data = resp.json()
    prompt_id = prompt_data.get("prompt", {}).get("id")
    print(f"Created Prompt: {prompt_id}")
    CREATED_PROMPTS.append({"id": prompt_id, "owner_id": user1_id})

    # 4. Get Prompt (User 1)
    resp = requests.get(f"http://localhost:8080/api/v1/prompts/{prompt_id}")
    if resp.status_code != 200:
        print(f"Get Prompt failed: {resp.status_code} - {resp.text}")
        return False
    if resp.json().get("prompt", {}).get("id") != prompt_id:
        print("Get Prompt returned wrong ID")
        return False

    # 5. List Prompts (User 1)
    resp = requests.get(f"http://localhost:8080/api/v1/prompts?owner_id={user1_id}")
    if resp.status_code != 200:
        print(f"List Prompts failed: {resp.status_code} - {resp.text}")
        return False
    prompts = resp.json().get("prompts", [])
    if not any(p["id"] == prompt_id for p in prompts):
        print("List Prompts did not find created prompt")
        return False

    # 6. Delete Prompt (User 1)
    resp = requests.delete(f"http://localhost:8080/api/v1/prompts/{prompt_id}?owner_id={user1_id}", headers=headers)
    if resp.status_code != 200:
        print(f"Delete Prompt failed: {resp.status_code} - {resp.text}")
        return False

    # 7. Verify Deletion
    resp = requests.get(f"http://localhost:8080/api/v1/prompts/{prompt_id}")
    if resp.status_code != 404:
        print(f"Prompt still exists after deletion or wrong status: {resp.status_code}")
        return False

    print("--- Prompt Lifecycle Test Passed ---")
    return True

def test_stats():
    print("--- Starting Stats Test ---")

    # Create a template with specific category and tags
    owner_id = f"stats_user_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}
    payload = {
        "owner_id": owner_id,
        "title": "Stats Template",
        "description": "Desc",
        "visibility": "VISIBILITY_PUBLIC",
        "type": "TEMPLATE_TYPE_USER",
        "tags": ["stats_tag_1", "stats_tag_2"],
        "category": "stats_cat"
    }
    resp = requests.post(BASE_URL, json=payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create Template failed: {resp.status_code}")
        return False

    tid = resp.json()["template"]["id"]
    CREATED_TEMPLATES.append({"id": tid, "owner_id": owner_id})

    # Test Categories
    print("Testing Categories...")
    resp = requests.get("http://localhost:8080/api/v1/categories")
    if resp.status_code != 200:
        print(f"List Categories failed: {resp.status_code}")
        return False
    cats = resp.json().get("categories", [])
    found_cat = False
    for c in cats:
        if c["name"] == "stats_cat":
            found_cat = True
            if c["count"] < 1:
                print(f"Category count incorrect: {c['count']}")
                return False
            break
    if not found_cat:
        print("Created category not found in stats")
        return False

    # Test Private Categories (Fix verification)
    print("Testing Private Categories...")
    private_cat = f"priv_cat_{int(time.time())}"
    payload_private = {
        "owner_id": owner_id,
        "title": "Private Stats Template",
        "description": "Desc",
        "visibility": "VISIBILITY_PRIVATE",
        "type": "TEMPLATE_TYPE_USER",
        "tags": ["private_tag"],
        "category": private_cat
    }
    resp = requests.post(BASE_URL, json=payload_private, headers=headers)
    if resp.status_code != 200:
         print("Failed to create private template")
         return False

    tid_priv = resp.json()["template"]["id"]
    CREATED_TEMPLATES.append({"id": tid_priv, "owner_id": owner_id})

    # 1. Query Public (should NOT see private_cat)
    resp = requests.get("http://localhost:8080/api/v1/categories")
    cats = resp.json().get("categories", [])
    for c in cats:
        if c["name"] == private_cat:
            print("Error: Private category visible in public list")
            return False

    # 2. Query Private (with owner_id)
    resp = requests.get(f"http://localhost:8080/api/v1/categories?owner_id={owner_id}")
    cats = resp.json().get("categories", [])
    found_private = False
    for c in cats:
        if c["name"] == private_cat:
            found_private = True
            break
    if not found_private:
        print("Error: Private category NOT found in authenticated owner list")
        return False
    print("Private category stats check passed.")

    # Test Tags
    print("Testing Tags...")
    resp = requests.get("http://localhost:8080/api/v1/tags")
    if resp.status_code != 200:
        print(f"List Tags failed: {resp.status_code}")
        return False
    tags = resp.json().get("tags", [])
    found_tag = False
    for t in tags:
        if t["name"] == "stats_tag_1":
            found_tag = True
            if t["count"] < 1:
                print(f"Tag count incorrect: {t['count']}")
                return False
            break
    if not found_tag:
        print("Created tag not found in stats")
        return False

    print("--- Stats Test Passed ---")
    return True

def test_pagination():
    print("--- Starting Pagination Test ---")
    owner_id = f"user_page_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}

    # Create 3 templates
    for i in range(3):
        payload = {
            "owner_id": owner_id,
            "title": f"Page Template {i}",
            "description": "Pagination test",
            "visibility": "VISIBILITY_PUBLIC",
            "type": "TEMPLATE_TYPE_USER",
            "tags": ["page_test"],
            "category": "page_cat"
        }
        resp = requests.post(BASE_URL, json=payload, headers=headers)
        if resp.status_code == 200:
            CREATED_TEMPLATES.append({"id": resp.json()["template"]["id"], "owner_id": owner_id})

    # Request page 1 (size 2)
    params = {"page_size": 2, "owner_id": owner_id}
    resp = requests.get(BASE_URL, params=params)
    if resp.status_code != 200:
        print(f"List Page 1 failed: {resp.status_code}")
        return False
    data = resp.json()
    if len(data.get("templates", [])) != 2:
        print(f"Page 1 size incorrect: {len(data.get('templates', []))}")
        return False

    # Request page 2 (size 2, offset 2)
    # Frontend sends page_token as stringified offset
    params = {"page_size": 2, "owner_id": owner_id, "page_token": "2"}
    resp = requests.get(BASE_URL, params=params)
    if resp.status_code != 200:
        print(f"List Page 2 failed: {resp.status_code}")
        return False
    data = resp.json()
    if len(data.get("templates", [])) != 1:
        print(f"Page 2 size incorrect: {len(data.get('templates', []))}")
        return False

    print("--- Pagination Test Passed ---")
    return True


def test_version_and_prompt():
    print("--- Starting Version & Prompt Test ---")
    owner_id = f"user_vp_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}

    # 1. Create Template with Content
    print("1. Creating Template with Content...")
    create_payload = {
        "owner_id": owner_id,
        "title": "Versioned Template",
        "description": "Testing versions",
        "visibility": "VISIBILITY_PUBLIC",
        "type": "TEMPLATE_TYPE_USER",
        "content": "Hello $$"
    }
    resp = requests.post(BASE_URL, json=create_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create failed: {resp.status_code} - {resp.text}")
        return False

    data = resp.json()
    template_id = data.get("template", {}).get("id")
    version_id_1 = data.get("version", {}).get("id")
    print(f"Created Template ID: {template_id}, Version 1 ID: {version_id_1}")
    CREATED_TEMPLATES.append({"id": template_id, "owner_id": owner_id})

    # 2. Update Template (New Version)
    print("2. Updating Content (Creating v2)...")
    update_payload = {
        "owner_id": owner_id,
        "template_id": template_id,

        "template_id": template_id,
        "owner_id": owner_id,
        "content": "Hello $$, how are you?"
    }
    resp = requests.put(f"{BASE_URL}/{template_id}", json=update_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Update failed: {resp.status_code} - {resp.text}")
        return False

    version_id_2 = resp.json().get("new_version", {}).get("id")
    print(f"Created Version 2 ID: {version_id_2}")

    if version_id_2 == version_id_1:
        print("Update did not create new version ID")
        return False

    # 3. List Versions
    print("3. Listing Versions...")
    resp = requests.get(f"{BASE_URL}/{template_id}/versions")
    if resp.status_code != 200:
        print(f"List Versions failed: {resp.status_code} - {resp.text}")
        return False

    versions = resp.json().get("versions", [])
    if len(versions) != 2:
        print(f"Expected 2 versions, got {len(versions)}")
        return False
    print("Versions listed successfully.")

    # 4. Create Prompt
    print("4. Creating Prompt...")
    PROMPT_URL = "http://localhost:8080/api/v1/prompts"
    prompt_payload = {
        "template_id": template_id,
        "version_id": version_id_2,
        "owner_id": owner_id,
        "variables": ["World"]
    }
    resp = requests.post(PROMPT_URL, json=prompt_payload, headers=headers)
    if resp.status_code != 200:
        print(f"Create Prompt failed: {resp.status_code} - {resp.text}")
        return False
    prompt_id = resp.json().get("prompt", {}).get("id")
    print(f"Created Prompt ID: {prompt_id}")
    CREATED_PROMPTS.append({"id": prompt_id, "owner_id": owner_id})

    # 5. List Prompts (Filter by Template)
    print("5. Listing Prompts for Template...")
    resp = requests.get(f"{PROMPT_URL}?template_id={template_id}")
    if resp.status_code != 200:
        print(f"List Prompts failed: {resp.status_code} - {resp.text}")
        return False
    prompts = resp.json().get("prompts", [])

    # Needs to match what we just created.
    # Note: Previous tests or runs might have created prompts for other templates,
    # but we are filtering by template_id.
    found = False
    for p in prompts:
        if p["id"] == prompt_id:
            found = True
            break

    if not found:
        print("Created prompt not found in list filtered by template_id")
        return False

    print("Prompts listed successfully.")

    # 6. Delete Prompt
    print("6. Deleting Prompt...")
    resp = requests.delete(f"{PROMPT_URL}/{prompt_id}?owner_id={owner_id}")
    if resp.status_code != 200:
        print(f"Delete Prompt failed: {resp.status_code} - {resp.text}")
        return False
    print("Prompt deleted successfully.")

    print("--- Version & Prompt Test Passed ---")
    return True

def test_tag_filtering():
    print("--- Starting Tag Filtering Test ---")

    owner_id = f"user_tags_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}

    # Create templates with different tags
    templates_data = [
        {"title": "TagTest_Python", "tags": ["python", "coding"], "visibility": "VISIBILITY_PUBLIC"},
        {"title": "TagTest_Java", "tags": ["java", "coding"], "visibility": "VISIBILITY_PUBLIC"},
        {"title": "TagTest_Snake", "tags": ["python", "reptile"], "visibility": "VISIBILITY_PUBLIC"},
        {"title": "TagTest_Private_Python", "tags": ["python", "secret"], "visibility": "VISIBILITY_PRIVATE"},
    ]

    created_ids = {}
    for t_data in templates_data:
        payload = {
            "owner_id": owner_id,
            "title": t_data["title"],
            "description": "Tag testing",
            "visibility": t_data["visibility"],
            "type": "TEMPLATE_TYPE_USER",
            "tags": t_data["tags"],
            "category": "test"
        }
        resp = requests.post(BASE_URL, json=payload, headers=headers)
        if resp.status_code != 200:
            print(f"Failed to create template {t_data['title']}: {resp.text}")
            return False

        tid = resp.json()["template"]["id"]
        created_ids[t_data["title"]] = tid
        CREATED_TEMPLATES.append({"id": tid, "owner_id": owner_id})

    time.sleep(1) # Allow for consistency

    # Test 1: Search 'python'. Should match Python & Snake. Should NOT match Java.
    print("Testing filter matches 'python'...")
    resp = requests.get(BASE_URL, params={"tags": "python"}, headers=headers)
    if resp.status_code != 200:
        print(f"Filter request failed: {resp.text}")
        return False

    data = resp.json()
    returned_templates = data.get("templates", []) + data.get("private_templates", [])
    returned_ids = [t["id"] for t in returned_templates]

    if created_ids["TagTest_Python"] not in returned_ids:
        print("Filter 'python' failed: Did not find TagTest_Python")
        return False
    if created_ids["TagTest_Snake"] not in returned_ids:
        print("Filter 'python' failed: Did not find TagTest_Snake")
        return False
    if created_ids["TagTest_Private_Python"] not in returned_ids:
        print("Filter 'python' failed: Did not find TagTest_Private_Python")
        return False
    if created_ids["TagTest_Java"] in returned_ids:
        print("Filter 'python' failed: Found TagTest_Java (should be excluded)")
        return False

    print("Filter 'python' passed.")

    # Test 2: Search 'coding'. Should match Python & Java. Not Snake.
    print("Testing filter matches 'coding'...")
    resp = requests.get(BASE_URL, params={"tags": "coding"}, headers=headers)
    if resp.status_code != 200:
        return False

    returned_templates = resp.json().get("templates", [])
    returned_ids = [t["id"] for t in returned_templates]

    if created_ids["TagTest_Python"] not in returned_ids:
        print("Filter 'coding' failed: Did not find TagTest_Python")
        return False
    if created_ids["TagTest_Java"] not in returned_ids:
        print("Filter 'coding' failed: Did not find TagTest_Java")
        return False
    if created_ids["TagTest_Snake"] in returned_ids:
        print("Filter 'coding' failed: Found TagTest_Snake (should be excluded)")
        return False

    print("Filter 'coding' passed.")

    # Test 3: Bracket Syntax 'tags[]=java'
    print("Testing bracket syntax 'tags[]=java'...")
    resp = requests.get(BASE_URL, params={"tags[]": "java"}, headers=headers)

    if resp.status_code != 200:
        print(f"Filter request failed: {resp.text}")
        return False

    data = resp.json()
    returned_templates = data.get("templates", []) + data.get("private_templates", [])
    returned_ids = [t["id"] for t in returned_templates]

    if created_ids["TagTest_Java"] not in returned_ids:
        print("Filter 'tags[]=java' failed: Did not find TagTest_Java")
        return False
    if created_ids["TagTest_Python"] in returned_ids:
        print("Filter 'tags[]=java' failed: Found TagTest_Python (unexpected)")
        return False
    print("Filter 'tags[]=java' passed.")

    return True

def test_likes_and_favorites():
    print("\n--- Starting Likes and Favorites Test ---")

    # 1. Setup specific users
    owner_id = f"owner_{int(time.time())}"
    token_owner = get_auth_token(owner_id)
    headers_owner = {"Authorization": f"Bearer {token_owner}"}

    user_id = f"liker_{int(time.time())}"
    token_user = get_auth_token(user_id)
    headers_user = {"Authorization": f"Bearer {token_user}"}

    # 2. Owner creates a template
    print(f"Creating template by {owner_id}...")
    t_data = {
        "title": "Social Template",
        "description": "To be liked",
        "content": "Like me",
        "owner_id": owner_id,
        "visibility": "VISIBILITY_PUBLIC",
        "category": "Social",
        "tags": ["social"]
    }
    resp = requests.post(BASE_URL, json=t_data, headers=headers_owner)
    if resp.status_code != 200:
        print(f"Failed to create template: {resp.text}")
        return False

    t_id = resp.json()["template"]["id"]
    CREATED_TEMPLATES.append({'id': t_id, 'owner_id': owner_id})
    print(f"Template created: {t_id}")

    # 3. User likes the template
    print(f"User {user_id} liking template...")
    resp = requests.post(f"{BASE_URL}/{t_id}/like", headers=headers_user)
    if resp.status_code != 200:
        print(f"Failed to like: {resp.text}")
        return False

    if not resp.json()["is_liked"]:
        print("Response should say is_liked=true")
        return False

    print("Like successful")

    # 4. Verify Get Template has correct stats
    print("Verifying stats in GetTemplate...")
    resp = requests.get(f"{BASE_URL}/{t_id}", headers=headers_user)
    t = resp.json()["template"]
    if not t["is_liked"]:
        print("GetTemplate: is_liked should be true")
        return False
    if t["like_count"] != 1:
        print(f"GetTemplate: like_count should be 1, got {t['like_count']}")
        return False

    # 5. List with my_likes
    print("Listing My Likes...")
    resp = requests.get(f"{BASE_URL}?my_likes=true", headers=headers_user)
    templates = resp.json().get("templates", [])
    if len(templates) == 0:
        print("List My Likes: Did not return the liked template (Empty List)")
        return False
    # Note: Template might not be the first one if parallel tests run, but filtering by my_likes should only show liked ones.
    found = False
    for tmpl in templates:
        if tmpl["id"] == t_id:
            found = True
            break
    if not found:
        print("List My Likes: Did not return the liked template")
        return False
    print("Found in My Likes")

    # 6. User favorites the template
    print(f"User {user_id} favoriting template...")
    resp = requests.post(f"{BASE_URL}/{t_id}/favorite", headers=headers_user)
    if resp.status_code != 200:
        print(f"Failed to favorite: {resp.text}")
        return False

    # 7. List with my_favorites
    print("Listing My Favorites...")
    resp = requests.get(f"{BASE_URL}?my_favorites=true", headers=headers_user)
    templates = resp.json().get("templates", [])
    if len(templates) == 0:
        print("List My Favorites: returned empty list")
        return False
    found = False
    for tmpl in templates:
        if tmpl["id"] == t_id:
            found = True
            break
    if not found:
        print("List My Favorites: Did not return the favorited template")
        return False
    print("Found in My Favorites")

    # 8. Unlike
    print("Unliking...")
    resp = requests.post(f"{BASE_URL}/{t_id}/like", headers=headers_user)
    if resp.json()["is_liked"]:
         print("Response should say is_liked=false")
         return False

    resp = requests.get(f"{BASE_URL}/{t_id}", headers=headers_user)
    if resp.json()["template"]["like_count"] != 0:
        print("Likes count should be 0")
        return False

    print("--- Likes and Favorites Test Passed ---")
    return True

def test_profile_update():
    print("--- Starting Profile Update Test ---")
    user_id = f"user_update_{int(time.time())}"
    token = get_auth_token(user_id)
    headers = {"Authorization": f"Bearer {token}"}

    update_url = "http://localhost:8080/api/v1/profile"

    # Update Profile
    update_data = {
        "display_name": "Updated Name",
        "avatar": "data:image/png;base64,fake"
    }
    resp = requests.put(update_url, headers=headers, json=update_data)
    if resp.status_code != 200:
        print(f"FAILED: Profile update failed with {resp.status_code}: {resp.text}")
        return False

    data = resp.json()
    if data['display_name'] != "Updated Name":
        print(f"FAILED: Display name mismatch. Expected 'Updated Name', got {data['display_name']}")
        return False
    if data['avatar'] != "data:image/png;base64,fake":
        print(f"FAILED: Avatar mismatch.")
        return False

    print("PASSED: Profile Update Test")
    return True

def main():
    # Start containers
    print("Starting containers...")
    if not run_command("docker compose -f deployment/docker-compose.yml up -d --build"):
        sys.exit(1)

    # Wait for backend
    print("Waiting for backend...")
    if not wait_for_service(BASE_URL):
        print("Backend failed to start or is not reachable.")
        run_command("docker compose -f deployment/docker-compose.yml logs backend")
        # run_command("docker compose -f deployment/docker-compose.yml down")
        sys.exit(1)

    # Run tests
    print("Running tests...")
    success = True
    if success: success = test_lifecycle()
    if success: success = test_auth()
    if success: success = test_prompt_lifecycle()
    if success: success = test_version_and_prompt()
    if success: success = test_stats()
    if success: success = test_tag_filtering()
    if success: success = test_pagination()
    if success: success = test_likes_and_favorites()
    if success: success = test_profile_update()

    # Cleanup is handled by atexit

    if success:
        print("All tests passed!")
        sys.exit(0)
    else:
        print("Tests failed!")
        sys.exit(1)

if __name__ == "__main__":
    main()

def test_template_alias_flow():
    app_url = "http://localhost:8080"
    owner_id = f"test_user_{int(time.time())}"
    token = get_auth_token(owner_id)
    headers = {"Authorization": f"Bearer {token}"}
    """Test the complete flow of aliases: Create template -> Generate versions -> Create alias -> Retrieve by alias"""

    # 1. Create a template (Version 1)
    template_payload = {
        "owner_id": owner_id,
        "type": "TEMPLATE_TYPE_USER",

        "title": "Alias Test Template",
        "description": "Template for testing aliases",
        "content": "This is version 1",
        "variables": ["var1"],
        "category": "developer",
        "visibility": "VISIBILITY_PUBLIC"
    }
    r = requests.post(f"{app_url}/api/v1/templates", json=template_payload, headers=headers)
    assert r.status_code == 200, r.text
    template_id = r.json().get("template", {}).get("id")
    print("CREATED TEMPLATE ID", template_id)

    # 2. Update template (Version 2)
    update_payload = {
        "owner_id": owner_id,
        "template_id": template_id,

        "title": "Alias Test Template",
        "description": "Template for testing aliases updated",
        "content": "This is version 2: {{var1}}",
        "variables": ["var1"],
        "category": "developer",
        "visibility": "VISIBILITY_PUBLIC"
    }
    r = requests.put(f"{app_url}/api/v1/templates/{template_id}", json=update_payload, headers=headers)
    assert r.status_code == 200, r.text
    v2_id = r.json().get("new_version", {}).get("id")

    # Check automatically created 'latest' alias
    r = requests.get(f"{app_url}/api/v1/templates/{template_id}/aliases/latest", headers=headers)
    assert r.status_code == 200, f"Expected 200, got {r.status_code}: {r.text}"
    assert r.json().get("id") == v2_id

    # 3. Create a new alias point to version 1
    # First, list versions to find version 1 id
    r = requests.get(f"{app_url}/api/v1/templates/{template_id}/versions", headers=headers)
    versions = r.json().get("versions", [])
    print("VERSIONS ARE:", versions)
    v1_id = next((v["id"] for v in versions if v.get("version") == 1), None)

    alias_payload = {
        "alias_name": "prod",
        "version_id": v1_id
    }
    r = requests.post(f"{app_url}/api/v1/templates/{template_id}/aliases", json=alias_payload, headers=headers)
    assert r.status_code == 200, r.text

    # 4. Agent retrieves prompt by alias
    r = requests.get(f"{app_url}/api/v1/templates/{template_id}/aliases/prod", headers=headers)
    assert r.status_code == 200, r.text
    assert "This is version 1" in r.json().get("content", ""), f"Content mismatch: {r.json()}"
