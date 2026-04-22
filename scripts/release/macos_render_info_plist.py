#!/usr/bin/env python3
import pathlib
import sys


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
        "{{VERSION}}": version,
        "{{SPARKLE_FEED_URL}}": feed_url,
        "{{SPARKLE_PUBLIC_ED_KEY}}": public_ed_key,
        "{{SPARKLE_AUTOCHECK}}": autocheck,
    }
    for key, value in replacements.items():
        rendered = rendered.replace(key, value)
    output_path.write_text(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
