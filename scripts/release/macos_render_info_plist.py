#!/usr/bin/env python3
import pathlib
import sys
from xml.sax.saxutils import escape


def xml_escape(value: str) -> str:
    return escape(value, {'"': "&quot;", "'": "&apos;"})


def main() -> int:
    if len(sys.argv) != 7:
        print(
            "usage: macos_render_info_plist.py <template> <output> <version> <feed-url> <public-ed-key> <autocheck>",
            file=sys.stderr,
        )
        return 2

    template_path = pathlib.Path(sys.argv[1])
    output_path = pathlib.Path(sys.argv[2])
    version = sys.argv[3]
    feed_url = sys.argv[4]
    public_ed_key = sys.argv[5]
    autocheck = sys.argv[6]

    rendered = template_path.read_text()
    replacements = {
        "{{VERSION}}": xml_escape(version),
        "{{SPARKLE_FEED_URL}}": xml_escape(feed_url),
        "{{SPARKLE_PUBLIC_ED_KEY}}": xml_escape(public_ed_key),
        "{{SPARKLE_AUTOCHECK}}": autocheck,
    }
    for key, value in replacements.items():
        rendered = rendered.replace(key, value)
    output_path.write_text(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
