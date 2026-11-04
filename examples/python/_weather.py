"""The one tool the tool-calling examples give their model. It returns canned
data so the examples run offline and are deterministic."""
import json

NAME = "get_weather"
DESCRIPTION = "Get the current weather for a city."
QUESTION = "What's the weather in Paris and in Tokyo right now, and which is warmer?"
SCHEMA = {
    "type": "object",
    "properties": {"city": {"type": "string", "description": "City name, e.g. Paris"}},
    "required": ["city"],
}

_CONDITIONS = {"paris": (14, "light rain"), "tokyo": (19, "clear"), "london": (11, "overcast")}


class ToolError(Exception):
    pass


def call(name: str, args: dict) -> str:
    """Run a tool call and return its JSON result; raise ToolError on bad input."""
    if name != NAME:
        raise ToolError(f"unknown tool {name!r}")
    city = args.get("city") if isinstance(args, dict) else None
    if not city or city.lower() not in _CONDITIONS:
        raise ToolError(f"get_weather: no data for {city!r}")
    temp, sky = _CONDITIONS[city.lower()]
    return json.dumps({"city": city, "temp_c": temp, "sky": sky})
