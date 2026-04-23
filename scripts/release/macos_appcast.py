#!/usr/bin/env python3
import pathlib
import sys
from xml.sax.saxutils import escape


def xml_escape(value: str) -> str:
    return escape(value, {'"': "&quot;", "'": "&apos;"})


def parse_metadata(path: pathlib.Path) -> dict[str, str]:
    values: dict[str, str] = {}
    for line in path.read_text().splitlines():
        if not line or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key] = value
    return values


def main() -> int:
    if len(sys.argv) != 8:
        print(
            "usage: macos_appcast.py <template> <output> <app-name> <appcast-url> <download-url> <public-ed-key> <metadata>",
            file=sys.stderr,
        )
        return 2

    template_path = pathlib.Path(sys.argv[1])
    output_path = pathlib.Path(sys.argv[2])
    app_name = sys.argv[3]
    appcast_url = sys.argv[4]
    download_url = sys.argv[5]
    public_ed_key = sys.argv[6]
    metadata_path = pathlib.Path(sys.argv[7])

    metadata = parse_metadata(metadata_path)
    mapping = {
        "{{APP_NAME}}": xml_escape(app_name),
        "{{APPCAST_URL}}": xml_escape(appcast_url),
        "{{VERSION}}": xml_escape(metadata["VERSION"]),
        "{{PUBLICATION_DATE}}": xml_escape(metadata["PUBLICATION_DATE"]),
        "{{DOWNLOAD_URL}}": xml_escape(download_url),
        "{{DOWNLOAD_LENGTH}}": xml_escape(metadata["ARCHIVE_LENGTH"]),
        "{{EDDSA_SIGNATURE}}": xml_escape(metadata.get("EDDSA_SIGNATURE", "UNSIGNED")),
        "{{SPARKLE_PUBLIC_ED_KEY}}": xml_escape(public_ed_key),
    }

    rendered = template_path.read_text()
    for key, value in mapping.items():
        rendered = rendered.replace(key, value)

    output_path.parent.mkdir(parents=True, exist_ok=True)
    output_path.write_text(rendered)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
