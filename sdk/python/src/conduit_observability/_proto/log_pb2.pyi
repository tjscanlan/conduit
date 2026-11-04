import datetime

from google.protobuf import timestamp_pb2 as _timestamp_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf.internal import enum_type_wrapper as _enum_type_wrapper
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class LogSeverity(int, metaclass=_enum_type_wrapper.EnumTypeWrapper):
    __slots__ = ()
    LOG_SEVERITY_UNSPECIFIED: _ClassVar[LogSeverity]
    LOG_SEVERITY_DEBUG: _ClassVar[LogSeverity]
    LOG_SEVERITY_INFO: _ClassVar[LogSeverity]
    LOG_SEVERITY_WARN: _ClassVar[LogSeverity]
    LOG_SEVERITY_ERROR: _ClassVar[LogSeverity]
LOG_SEVERITY_UNSPECIFIED: LogSeverity
LOG_SEVERITY_DEBUG: LogSeverity
LOG_SEVERITY_INFO: LogSeverity
LOG_SEVERITY_WARN: LogSeverity
LOG_SEVERITY_ERROR: LogSeverity

class LogRecord(_message.Message):
    __slots__ = ("timestamp", "severity", "body", "attributes", "trace_id", "span_id")
    class AttributesEntry(_message.Message):
        __slots__ = ("key", "value")
        KEY_FIELD_NUMBER: _ClassVar[int]
        VALUE_FIELD_NUMBER: _ClassVar[int]
        key: str
        value: str
        def __init__(self, key: _Optional[str] = ..., value: _Optional[str] = ...) -> None: ...
    TIMESTAMP_FIELD_NUMBER: _ClassVar[int]
    SEVERITY_FIELD_NUMBER: _ClassVar[int]
    BODY_FIELD_NUMBER: _ClassVar[int]
    ATTRIBUTES_FIELD_NUMBER: _ClassVar[int]
    TRACE_ID_FIELD_NUMBER: _ClassVar[int]
    SPAN_ID_FIELD_NUMBER: _ClassVar[int]
    timestamp: _timestamp_pb2.Timestamp
    severity: LogSeverity
    body: str
    attributes: _containers.ScalarMap[str, str]
    trace_id: str
    span_id: str
    def __init__(self, timestamp: _Optional[_Union[datetime.datetime, _timestamp_pb2.Timestamp, _Mapping]] = ..., severity: _Optional[_Union[LogSeverity, str]] = ..., body: _Optional[str] = ..., attributes: _Optional[_Mapping[str, str]] = ..., trace_id: _Optional[str] = ..., span_id: _Optional[str] = ...) -> None: ...
