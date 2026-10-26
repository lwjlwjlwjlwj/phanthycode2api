#!/usr/bin/env python3
"""Push the reverse-engineered OpenAPI bundle to Apifox.

Default behaviour is a local dry run: the spec is validated and a summary is
printed, but no network request is made. A real push happens only when --push
is passed explicitly and APIFOX_ACCESS_TOKEN is present in the environment.

The access token is read from APIFOX_ACCESS_TOKEN only -- never from argv --
so it does not leak into shell history or process listings.

Standard library only (Python 3.8+).
"""

import argparse
import json
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

DEFAULT_SPEC = Path(__file__).resolve().with_name("openapi.json")
DEFAULT_API_BASE = "https://api.apifox.com"
# Documented at https://apifox.com/apidoc/docs-site/4478210/api-173409873
# (fetched 2026-10-26): POST /v1/projects/{projectId}/import-openapi
DEFAULT_IMPORT_PATH = "/v1/projects/{project_id}/import-openapi"
DEFAULT_API_VERSION = "2024-03-28"

HTTP_METHODS = {
    "get", "put", "post", "delete", "options", "head", "patch", "trace",
}

# JWT-shaped material must never appear in a deliverable spec.
RE_JWT = re.compile(r"eyJ[A-Za-z0-9_-]{10,}")
# A literal bearer credential (anything that is not a <placeholder>).
RE_BEARER = re.compile(r"(?i)\bbearer\s+([A-Za-z0-9._~+/=-]{20,})")
# OpenAPI response keys: status code, range (2XX), or default.
RE_RESPONSE_KEY = re.compile(r"^(default|[1-5](?:[0-9]{2}|XX))$")


# --------------------------------------------------------------------------
# Validation
# --------------------------------------------------------------------------

def iter_operations(doc):
    """Yield (path, method, operation_object) for every path item method."""
    paths = doc.get("paths")
    if not isinstance(paths, dict):
        return
    for path, item in paths.items():
        if not isinstance(item, dict):
            continue
        for method, op in item.items():
            if method.lower() in HTTP_METHODS and isinstance(op, dict):
                yield path, method.lower(), op


def collect_refs(node, location="#"):
    """Yield ($ref value, location) for every $ref in the document."""
    if isinstance(node, dict):
        for key, value in node.items():
            here = "%s/%s" % (location, key)
            if key == "$ref" and isinstance(value, str):
                yield value, here
            else:
                for found in collect_refs(value, here):
                    yield found
    elif isinstance(node, list):
        for index, value in enumerate(node):
            for found in collect_refs(value, "%s/%d" % (location, index)):
                yield found


def resolve_pointer(doc, ref):
    """Resolve a local JSON pointer. Returns (ok, value_or_error)."""
    if not ref.startswith("#"):
        return False, "non-local $ref is not allowed: %s" % ref
    pointer = ref[1:]
    if pointer == "":
        return True, doc
    if not pointer.startswith("/"):
        return False, "malformed JSON pointer: %s" % ref
    current = doc
    for raw_token in pointer.split("/")[1:]:
        token = raw_token.replace("~1", "/").replace("~0", "~")
        if isinstance(current, dict):
            if token not in current:
                return False, "unresolved $ref: %s (missing %r)" % (ref, token)
            current = current[token]
        elif isinstance(current, list):
            try:
                index = int(token)
            except ValueError:
                return False, "unresolved $ref: %s (bad index %r)" % (ref, token)
            if index < 0 or index >= len(current):
                return False, "unresolved $ref: %s (index out of range)" % ref
            current = current[index]
        else:
            return False, "unresolved $ref: %s (scalar encountered)" % ref
    return True, current


def validate(doc):
    """Validate the spec structurally. Returns (errors, warnings, stats)."""
    errors = []
    warnings = []

    if not isinstance(doc, dict):
        return ["document root must be a JSON object"], warnings, {}

    version = doc.get("openapi")
    if not isinstance(version, str) or not version.startswith("3.1"):
        errors.append("openapi version must be a 3.1.x string, found: %r" % (version,))

    info = doc.get("info")
    if not isinstance(info, dict):
        errors.append("info object is missing")
    else:
        for field in ("title", "version"):
            if not isinstance(info.get(field), str) or not info.get(field):
                errors.append("info.%s must be a non-empty string" % field)

    paths = doc.get("paths")
    if not isinstance(paths, dict) or not paths:
        errors.append("paths object is missing or empty")

    operation_ids = []
    operation_refs = []
    for path, method, op in iter_operations(doc):
        operation_ids.append(op.get("operationId"))
        operation_refs.append("%s %s" % (method.upper(), path))

        if not isinstance(op.get("operationId"), str) or not op.get("operationId"):
            errors.append("%s %s has no operationId" % (method.upper(), path))
        if not isinstance(op.get("summary"), str) or not op.get("summary"):
            warnings.append("%s %s has no summary" % (method.upper(), path))
        responses = op.get("responses")
        if not isinstance(responses, dict) or not responses:
            errors.append("%s %s has no responses object" % (method.upper(), path))
        else:
            for response_key, response_value in responses.items():
                if not RE_RESPONSE_KEY.match(str(response_key)):
                    errors.append(
                        "%s %s has an invalid response key %r "
                        "(must be an HTTP status code, a range like 2XX, or 'default')"
                        % (method.upper(), path, response_key)
                    )
                if not isinstance(response_value, dict) or "description" not in response_value:
                    errors.append(
                        "%s %s response %r must be an object with a description"
                        % (method.upper(), path, response_key)
                    )

    seen = {}
    for op_id in operation_ids:
        if not isinstance(op_id, str):
            continue
        if op_id in seen:
            errors.append("duplicate operationId: %s" % op_id)
        seen[op_id] = True
    if len(operation_ids) != len(set(x for x in operation_ids if isinstance(x, str))):
        errors.append("operationIds are not unique")

    ref_count = 0
    for ref, location in collect_refs(doc):
        ref_count += 1
        ok, message = resolve_pointer(doc, ref)
        if not ok:
            errors.append("%s at %s" % (message, location))

    text = json.dumps(doc, ensure_ascii=False)
    for match in RE_JWT.finditer(text):
        errors.append("JWT-shaped secret in spec: %s..." % match.group(0)[:12])
    for match in RE_BEARER.finditer(text):
        value = match.group(1)
        if value.startswith("<"):
            continue
        errors.append("literal bearer credential in spec: %s..." % value[:8])

    tags = doc.get("tags")
    declared_tags = set()
    if isinstance(tags, list):
        for tag in tags:
            if isinstance(tag, dict) and isinstance(tag.get("name"), str):
                declared_tags.add(tag["name"])
    for path, method, op in iter_operations(doc):
        for tag in op.get("tags", []) or []:
            if declared_tags and tag not in declared_tags:
                warnings.append("%s %s uses undeclared tag %r"
                                % (method.upper(), path, tag))

    schemas = doc.get("components", {}).get("schemas", {})
    stats = {
        "openapi": version,
        "paths": len(paths) if isinstance(paths, dict) else 0,
        "operations": len(operation_ids),
        "operationIds": [x for x in operation_ids if isinstance(x, str)],
        "tags": sorted(declared_tags),
        "schemas": len(schemas) if isinstance(schemas, dict) else 0,
        "refs": ref_count,
    }
    return errors, warnings, stats


def print_summary(stats, spec_path, warnings):
    print("spec:        %s" % spec_path)
    print("openapi:     %s" % stats.get("openapi"))
    print("paths:       %s" % stats.get("paths"))
    print("operations:  %s" % stats.get("operations"))
    print("schemas:     %s" % stats.get("schemas"))
    print("local $refs: %s" % stats.get("refs"))
    print("tags:        %s" % ", ".join(stats.get("tags", [])))
    if stats.get("operationIds"):
        print("operationIds:")
        for op_id in stats["operationIds"]:
            print("  - %s" % op_id)
    if warnings:
        print("warnings (%d):" % len(warnings))
        for warning in warnings:
            print("  ! %s" % warning)


def load_spec(path):
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)


def run_self_test(spec_path):
    print("== self-test: %s ==" % spec_path)
    try:
        doc = load_spec(spec_path)
    except (OSError, ValueError) as exc:
        print("FAIL: cannot load spec: %s" % exc)
        return 1
    errors, warnings, stats = validate(doc)
    print_summary(stats, spec_path, warnings)
    if errors:
        print("FAIL: %d validation error(s):" % len(errors))
        for error in errors:
            print("  - %s" % error)
        return 1
    print("SELF-TEST OK")
    return 0


# --------------------------------------------------------------------------
# Push
# --------------------------------------------------------------------------

def build_import_path(api_base, import_path, project_id):
    template = import_path or DEFAULT_IMPORT_PATH
    quoted = urllib.parse.quote(str(project_id), safe="")
    if "{project_id}" in template:
        path = template.replace("{project_id}", quoted)
    elif "{projectId}" in template:
        path = template.replace("{projectId}", quoted)
    else:
        path = template
    return api_base.rstrip("/") + path


def push(doc, spec_text, args):
    token = os.environ.get("APIFOX_ACCESS_TOKEN", "").strip()
    if not token:
        print("ERROR: APIFOX_ACCESS_TOKEN is not set; refusing to push.", file=sys.stderr)
        print("       Export a system-level Apifox access token, then retry.", file=sys.stderr)
        return 2
    if not args.project_id:
        print("ERROR: --project-id is required for a push.", file=sys.stderr)
        return 2

    options = {
        # Conservative: merge into matching endpoints/models, never delete.
        "endpointOverwriteBehavior": "AUTO_MERGE",
        "schemaOverwriteBehavior": "AUTO_MERGE",
        "deleteUnmatchedResources": False,
        "prependBasePath": False,
        "updateFolderOfChangedEndpoint": False,
    }
    # Apifox documents `input` as either a URL object or a string payload.
    # We always send the spec text as a string (the documented default form).
    body = {"input": spec_text, "options": options}

    url = build_import_path(args.api_base, args.import_path, args.project_id)
    request = urllib.request.Request(
        url,
        data=json.dumps(body).encode("utf-8"),
        method="POST",
        headers={
            "Authorization": "Bearer %s" % token,
            "Content-Type": "application/json",
            "Accept": "application/json",
            "X-Apifox-Api-Version": args.api_version,
            "User-Agent": "phanthy-reverse-engineering-m4/1.0",
        },
    )

    print("POST %s" % url)
    print("options: %s" % json.dumps(options))
    try:
        with urllib.request.urlopen(request, timeout=args.timeout) as response:
            status = response.status
            raw = response.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as exc:
        body_text = exc.read().decode("utf-8", "replace")
        print("HTTP %s" % exc.code)
        print(body_text)
        print("ERROR: import rejected; see response body above.", file=sys.stderr)
        return 1
    except urllib.error.URLError as exc:
        print("ERROR: transport failure: %s" % exc.reason, file=sys.stderr)
        return 3

    print("HTTP %s" % status)
    print(raw)
    if not (200 <= status < 300):
        print("ERROR: non-2xx response.", file=sys.stderr)
        return 1
    return 0


# --------------------------------------------------------------------------
# CLI
# --------------------------------------------------------------------------

def build_parser():
    parser = argparse.ArgumentParser(
        description="Validate and (optionally) push the PhanthyCode OpenAPI "
                    "bundle to Apifox. Dry run is the default.",
        epilog="Token: APIFOX_ACCESS_TOKEN env var only. "
               "Example: APIFOX_ACCESS_TOKEN=<token> python3 push_apifox.py "
               "--spec openapi.json --project-id 123456 --push",
    )
    parser.add_argument("--spec", default=str(DEFAULT_SPEC),
                        help="path to openapi.json (default: alongside this script)")
    parser.add_argument("--project-id", default=os.environ.get("APIFOX_PROJECT_ID"),
                        help="Apifox project id (default: APIFOX_PROJECT_ID)")
    parser.add_argument("--api-base", default=os.environ.get("APIFOX_API_BASE", DEFAULT_API_BASE),
                        help="Apifox API base (default: %s or APIFOX_API_BASE)" % DEFAULT_API_BASE)
    parser.add_argument("--import-path", default=DEFAULT_IMPORT_PATH,
                        help="import path template; {project_id} is substituted "
                             "(default: %s)" % DEFAULT_IMPORT_PATH)
    parser.add_argument("--api-version", default=DEFAULT_API_VERSION,
                        help="X-Apifox-Api-Version header value (default: %s)" % DEFAULT_API_VERSION)
    mode = parser.add_argument_group("mode")
    mode.add_argument("--dry-run", action="store_true",
                      help="validate locally only (this is the default behaviour)")
    mode.add_argument("--push", action="store_true",
                      help="actually POST to Apifox; requires APIFOX_ACCESS_TOKEN")
    mode.add_argument("--self-test", action="store_true",
                      help="validate the bundled spec and exit 0 only if valid")
    parser.add_argument("--timeout", type=float, default=60.0,
                        help="HTTP timeout seconds (default: 60)")
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)

    if args.self_test:
        return run_self_test(DEFAULT_SPEC)

    if args.push and args.dry_run:
        print("ERROR: choose either --push or --dry-run, not both.", file=sys.stderr)
        return 2

    spec_path = Path(args.spec).expanduser()
    try:
        spec_text = spec_path.read_text(encoding="utf-8")
        doc = json.loads(spec_text)
    except (OSError, ValueError) as exc:
        print("ERROR: cannot load %s: %s" % (spec_path, exc), file=sys.stderr)
        return 1

    errors, warnings, stats = validate(doc)
    print_summary(stats, spec_path, warnings)
    if errors:
        print("FAIL: %d validation error(s):" % len(errors), file=sys.stderr)
        for error in errors:
            print("  - %s" % error, file=sys.stderr)
        return 1

    if not args.push:
        print("DRY RUN: spec is valid; no request sent. Add --push to import.")
        return 0

    return push(doc, spec_text, args)


if __name__ == "__main__":
    sys.exit(main())
