import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class SpanStatus(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    SPAN_STATUS_UNSET: _ClassVar[SpanStatus]
    SPAN_STATUS_OK: _ClassVar[SpanStatus]
    SPAN_STATUS_ERROR: _ClassVar[SpanStatus]
SPAN_STATUS_UNSET: SpanStatus
SPAN_STATUS_OK: SpanStatus
SPAN_STATUS_ERROR: SpanStatus

class Span(_message.Message):
    __slots__ = ("trace_id", "span_id", "parent_id", "name", "start_time", "end_time", "status", "attributes", "llm_call")
    class AttributesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    SPAN_ID_FIELD_NUMBER: _ClassVar[int]
    PARENT_ID_FIELD_NUMBER: _ClassVar[int]
    NAME_FIELD_NUMBER: _ClassVar[int]
    START_TIME_FIELD_NUMBER: _ClassVar[int]
    END_TIME_FIELD_NUMBER: _ClassVar[int]
    STATUS_FIELD_NUMBER: _ClassVar[int]
    ATTRIBUTES_FIELD_NUMBER: _ClassVar[int]
    LLM_CALL_FIELD_NUMBER: _ClassVar[int]
    trace_id: str
    span_id: str
    parent_id: str
    name: str
    start_time: _timestamp_pb2.Timestamp
    end_time: _timestamp_pb2.Timestamp
    status: SpanStatus
    attributes: _containers.ScalarMap[str, str]
    llm_call: LLMCallData
    def __init__(self, trace_id: _Optional[str] = ..., span_id: _Optional[str] = ..., parent_id: _Optional[str] = ..., name: _Optional[str] = ..., start_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., end_time: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., status: _Optional[_Union[SpanStatus, str]] = ..., attributes: _Optional[_Mapping[str, str]] = ..., llm_call: _Optional[_Union[LLMCallData, _Mapping]] = ...) -> None: ...

class LLMCallData(_message.Message):
    __slots__ = ("model", "prompt", "completion", "input_tokens", "output_tokens", "cost_usd")
    MODEL_FIELD_NUMBER: _ClassVar[int]
    PROMPT_FIELD_NUMBER: _ClassVar[int]
    COMPLETION_FIELD_NUMBER: _ClassVar[int]
    INPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    OUTPUT_TOKENS_FIELD_NUMBER: _ClassVar[int]
    COST_USD_FIELD_NUMBER: _ClassVar[int]
    model: str
    prompt: str
    completion: str
    input_tokens: int
    output_tokens: int
    cost_usd: float
    def __init__(self, model: _Optional[str] = ..., prompt: _Optional[str] = ..., completion: _Optional[str] = ..., input_tokens: _Optional[int] = ..., output_tokens: _Optional[int] = ..., cost_usd: _Optional[float] = ...) -> None: ...
