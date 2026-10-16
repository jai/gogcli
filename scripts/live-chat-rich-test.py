#!/usr/bin/env python3
"""Exercise native Chat commands using only messages created by this run.

Example (writes synthetic messages, then removes them):
  scripts/live-chat-rich-test.py --gog ./bin/gog --account you@example.com \
    --space spaces/SPACE --allow-write

The caller must verify the sole member is the supplied account before running.
The script additionally requires a space named exactly 'Gog CLI fork validation'
with one HUMAN membership. It never creates spaces or reads unrelated messages.
CardsV2 need an app-created fixture and are covered by offline command tests.
"""

import argparse
import json
import os
import re
import struct
import subprocess
import sys
import tempfile
import uuid
import zlib
from pathlib import Path


class SmokeFailure(Exception):
    """A diagnostic safe to include in the summary."""


class Smoke:
    def __init__(self, args):
        self.args = args
        self.checks = []
        self.created = []
        self.cleanup_errors = 0
        self.marker = "gog-rich-smoke-" + uuid.uuid4().hex

    def invoke(self, *args):
        # Do not inherit flags that could change output, account, or write guards.
        # Keep auth/keyring environment intact, including GOG_KEYRING_PASSWORD.
        env = os.environ.copy()
        for key in (
            "GOG_ACCOUNT", "GOG_JSON", "GOG_PLAIN", "GOG_WRAP_UNTRUSTED",
            "GOG_RESULTS_ONLY", "GOG_SELECT", "GOG_DRY_RUN", "GOG_READONLY",
        ):
            env.pop(key, None)
        try:
            return subprocess.run(
                [self.args.gog, "--account", self.args.account, "--json",
                 "--no-input", *args],
                capture_output=True, text=True, timeout=90, env=env,
            )
        except subprocess.TimeoutExpired:
            raise SmokeFailure("command_timeout; inspect validation space for an uncertain send") from None
        except OSError:
            raise SmokeFailure("cannot_execute_gog") from None

    @staticmethod
    def decode(result):
        if result.returncode:
            # Google error bodies can contain private text; never relay them.
            raise SmokeFailure("command_failed_exit_" + str(result.returncode))
        try:
            value = json.loads(result.stdout)
        except (ValueError, TypeError):
            raise SmokeFailure("invalid_json_output") from None
        if not isinstance(value, dict):
            raise SmokeFailure("unexpected_json_shape")
        return value

    def call(self, *args):
        return self.decode(self.invoke(*args))

    def passed(self, name):
        self.checks.append({"name": name, "status": "passed"})

    @staticmethod
    def require(condition, diagnostic):
        if not condition:
            raise SmokeFailure(diagnostic)

    def preflight(self):
        space = self.call(
            "api", "call", "chat", "v1", "chat.spaces.get",
            "--params", json.dumps({"name": self.args.space}),
            "--scope", "https://www.googleapis.com/auth/chat.spaces",
        )
        self.require(
            space.get("name") == self.args.space
            and space.get("displayName") == "Gog CLI fork validation"
            and space.get("spaceType") == "SPACE"
            and space.get("accessSettings", {}).get("accessState") == "PRIVATE",
            "refused_non_validation_space",
        )
        # Group memberships and invitees are omitted unless explicitly requested.
        memberships = self.call(
            "api", "call", "chat", "v1", "chat.spaces.members.list",
            "--params", json.dumps({
                "parent": self.args.space, "pageSize": 100,
                "showGroups": True, "showInvited": True,
            }),
            "--scope", "https://www.googleapis.com/auth/chat.memberships",
        )
        members = memberships.get("memberships", [])
        self.require(
            isinstance(members, list) and len(members) == 1
            and not memberships.get("nextPageToken")
            and members[0].get("member", {}).get("type") == "HUMAN"
            and members[0].get("state") == "JOINED"
            and members[0].get("role") == "ROLE_MANAGER"
            and not members[0].get("groupMember"),
            "refused_space_without_exactly_one_human_member",
        )
        self.passed("validation_space_preflight")

    def send(self, text, *extra):
        value = self.call("chat", "messages", "send", self.args.space,
                          "--text", text, *extra)
        message = value.get("message", {})
        name = message.get("name", "")
        self.require(
            isinstance(name, str)
            and re.fullmatch(re.escape(self.args.space) + r"/messages/[^/]+", name),
            "send_response_missing_resource; inspect validation space for an uncertain send",
        )
        self.created.append(name)
        return message

    def get(self, name, *extra):
        value = self.call("chat", "messages", "get", name, *extra)
        message = value.get("message")
        self.require(isinstance(message, dict) and message.get("name") == name,
                     "get_response_missing_message")
        return message

    def owned(self, name):
        self.require(name in self.created, "refused_mutation_of_unowned_message")

    def update(self, name, text, *extra):
        self.owned(name)
        return self.call(*extra, "chat", "messages", "update", name, "--text", text)

    def delete(self, name, *extra):
        self.owned(name)
        root_flags = [arg for arg in extra if arg in ("--dry-run", "--readonly")]
        local_flags = [arg for arg in extra if arg not in root_flags]
        return self.invoke("--force", *root_flags, "chat", "messages", "delete", name,
                           *local_flags)

    @staticmethod
    def is_missing(result):
        return result.returncode != 0 and bool(re.search(
            r"\b404\b|\bnotFound\b|\bnot_found\b|\bnot found\b",
            result.stderr, re.IGNORECASE,
        ))

    def require_missing(self, name):
        self.require(self.is_missing(self.invoke("chat", "messages", "get", name)),
                     "deleted_message_did_not_return_404")

    def exercise(self):
        text = self.marker + ": <https://example.com|validation link>"
        message = self.send(text)
        name = message["name"]
        fetched = self.get(name)
        formatted = fetched.get("formattedText", "")
        self.require("https://example.com" in formatted and "validation link" in formatted,
                     "get_dropped_named_hyperlink")
        self.passed("native_get_named_hyperlink")

        # Restrict listing to this run's new thread; do not fetch older messages.
        thread = message.get("thread", {}).get("name", "")
        self.require(re.fullmatch(re.escape(self.args.space) + r"/threads/[^/]+", thread),
                     "send_response_missing_thread")
        listed = self.call("chat", "messages", "list", self.args.space,
                           "--full", "--thread", thread, "--max", "100")
        full = [item for item in listed.get("messages", []) if item.get("name") == name]
        self.require(len(full) == 1 and full[0].get("formattedText") == formatted,
                     "full_list_dropped_named_hyperlink")
        self.passed("native_list_full_named_hyperlink")
        markdown = self.get(name, "--markup", "markdown").get("formattedText", "")
        self.require("https://example.com" in markdown and "validation link" in markdown,
                     "markdown_get_dropped_hyperlink")
        self.passed("native_get_markdown")

        dry_text = self.marker + " dry-run must not persist"
        plan = self.update(name, dry_text, "--dry-run")
        self.require(plan.get("dry_run") is True
                     and plan.get("op") == "chat.messages.update"
                     and self.get(name).get("text") == fetched.get("text"),
                     "dry_run_update_changed_message")
        plan = self.decode(self.delete(name, "--dry-run"))
        self.require(plan.get("dry_run") is True and plan.get("op") == "chat.messages.delete"
                     and self.get(name).get("text") == fetched.get("text"),
                     "dry_run_delete_changed_message")
        self.passed("dry_run_update_and_delete")

        blocked = self.invoke("--readonly", "chat", "messages", "update", name,
                              "--text", dry_text)
        self.require(blocked.returncode != 0 and "readonly" in blocked.stderr.lower()
                     and self.get(name).get("text") == fetched.get("text"),
                     "readonly_update_not_blocked")
        blocked = self.delete(name, "--readonly")
        self.require(blocked.returncode != 0 and "readonly" in blocked.stderr.lower()
                     and self.get(name).get("text") == fetched.get("text"),
                     "readonly_delete_not_blocked")
        self.passed("readonly_update_and_delete")

        replacement = self.marker + " edited successfully"
        self.update(name, replacement)
        self.require(self.get(name).get("text") == replacement,
                     "update_readback_mismatch")
        self.passed("native_update_exact_readback")
        self.decode(self.delete(name))
        self.require_missing(name)
        self.passed("native_delete_and_404")

        with tempfile.TemporaryDirectory(prefix="gog-rich-smoke-") as directory:
            image = Path(directory) / "validation.png"
            image.write_bytes(tiny_png())
            attachment = self.send(self.marker + " synthetic image", "--attach", str(image))
        image_name = attachment["name"]
        attachments = self.get(image_name).get("attachment", [])
        self.require(any(
            item.get("contentType") == "image/png"
            and (item.get("attachmentDataRef", {}).get("resourceName")
                 or item.get("downloadUri") or item.get("thumbnailUri"))
            for item in attachments
        ), "native_get_missing_image_metadata")
        self.passed("native_image_attachment_metadata")
        self.decode(self.delete(image_name))
        self.require_missing(image_name)

        root = self.send(self.marker + " synthetic root")
        root_name = root["name"]
        thread = root.get("thread", {}).get("name", "")
        self.require(re.fullmatch(re.escape(self.args.space) + r"/threads/[^/]+", thread),
                     "root_response_missing_thread")
        reply = self.send(self.marker + " synthetic reply", "--thread", thread)
        reply_name = reply["name"]
        self.require(reply.get("thread", {}).get("name") == thread,
                     "reply_created_in_different_thread")
        refused = self.delete(root_name)
        self.require(refused.returncode != 0 and bool(re.search(
            r"thread|repl|FAILED_PRECONDITION|\b400\b|\b409\b",
            refused.stderr, re.IGNORECASE,
        )), "root_delete_without_delete_replies_not_refused")
        self.get(root_name)
        self.get(reply_name)
        self.passed("global_force_preserves_thread_replies")
        self.decode(self.delete(root_name, "--delete-replies"))
        self.require_missing(root_name)
        self.require_missing(reply_name)
        self.passed("explicit_delete_replies_removes_run_owned_thread")

    def cleanup(self):
        for name in reversed(self.created):
            try:
                # Delete replies first; do not cascade during recovery cleanup.
                result = self.delete(name)
                if self.is_missing(result):
                    continue
                self.decode(result)
                self.require_missing(name)
            except SmokeFailure:
                self.cleanup_errors += 1
        self.checks.append({"name": "cleanup", "status":
                            "failed" if self.cleanup_errors else "passed"})


def tiny_png():
    """A valid opaque 1x1 PNG generated entirely with the standard library."""
    def chunk(kind, data):
        return (struct.pack(">I", len(data)) + kind + data
                + struct.pack(">I", zlib.crc32(kind + data)))
    return (b"\x89PNG\r\n\x1a\n"
            + chunk(b"IHDR", struct.pack(">IIBBBBB", 1, 1, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(b"\x00\x00\x80\xff\xff"))
            + chunk(b"IEND", b""))


def main():
    parser = argparse.ArgumentParser(description=__doc__,
                                     formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--gog", required=True, help="Built Gog binary to test")
    parser.add_argument("--account", required=True, help="Verified sole member's email")
    parser.add_argument("--space", required=True, help="Existing private validation space (spaces/ID)")
    parser.add_argument("--allow-write", action="store_true",
                        help="Explicitly permit synthetic send/update/delete tests and cleanup")
    args = parser.parse_args()
    if not args.allow_write:
        parser.error("--allow-write is required: this smoke test creates synthetic messages")
    if not re.fullmatch(r"spaces/[^/\s]+", args.space):
        parser.error("--space must be a full spaces/ID resource")
    smoke = Smoke(args)
    failure = None
    try:
        smoke.preflight()
        smoke.exercise()
    except SmokeFailure as exc:
        failure = str(exc)
    except KeyboardInterrupt:
        failure = "interrupted"
    except Exception:
        failure = "unexpected_local_error"
    finally:
        smoke.cleanup()
    summary = {
        "status": "failed" if failure or smoke.cleanup_errors else "passed",
        "checks": smoke.checks,
        "created_count": len(smoke.created),
        "cleanup_errors": smoke.cleanup_errors,
    }
    if failure:
        summary["error"] = failure
    print(json.dumps(summary, sort_keys=True))
    return 1 if failure or smoke.cleanup_errors else 0


if __name__ == "__main__":
    sys.exit(main())
