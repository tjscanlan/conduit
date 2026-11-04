from . import span_pb2 as _span_pb2
from . import metric_pb2 as _metric_pb2
from . import log_pb2 as _log_pb2
from google.protobuf.internal import containers as _containers
from google.protobuf import descriptor as _descriptor
from google.protobuf import message as _message
from collections.abc import Iterable as _Iterable, Mapping as _Mapping
from typing import ClassVar as _ClassVar, Optional as _Optional, Union as _Union

DESCRIPTOR: _descriptor.FileDescriptor

class IngestSpanRequest(_message.Message):
    __slots__ = ("span",)
    SPAN_FIELD_NUMBER: _ClassVar[int]
    span: _span_pb2.Span
    def __init__(self, span: _Optional[_Union[_span_pb2.Span, _Mapping]] = ...) -> None: ...

class IngestSpanResponse(_message.Message):
    __slots__ = ("accepted", "rejection_reason")
    ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    REJECTION_REASON_FIELD_NUMBER: _ClassVar[int]
    accepted: bool
    rejection_reason: str
    def __init__(self, accepted: _Optional[bool] = ..., rejection_reason: _Optional[str] = ...) -> None: ...

class IngestBatchRequest(_message.Message):
    __slots__ = ("spans", "metrics", "logs")
    SPANS_FIELD_NUMBER: _ClassVar[int]
    METRICS_FIELD_NUMBER: _ClassVar[int]
    LOGS_FIELD_NUMBER: _ClassVar[int]
    spans: _containers.RepeatedCompositeFieldContainer[_span_pb2.Span]
    metrics: _containers.RepeatedCompositeFieldContainer[_metric_pb2.Metric]
    logs: _containers.RepeatedCompositeFieldContainer[_log_pb2.LogRecord]
    def __init__(self, spans: _Optional[_Iterable[_Union[_span_pb2.Span, _Mapping]]] = ..., metrics: _Optional[_Iterable[_Union[_metric_pb2.Metric, _Mapping]]] = ..., logs: _Optional[_Iterable[_Union[_log_pb2.LogRecord, _Mapping]]] = ...) -> None: ...

class IngestBatchResponse(_message.Message):
    __slots__ = ("spans_accepted", "metrics_accepted", "logs_accepted", "errors")
    SPANS_ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    METRICS_ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    LOGS_ACCEPTED_FIELD_NUMBER: _ClassVar[int]
    ERRORS_FIELD_NUMBER: _ClassVar[int]
    spans_accepted: int
    metrics_accepted: int
    logs_accepted: int
    errors: _containers.RepeatedScalarFieldContainer[str]
    def __init__(self, spans_accepted: _Optional[int] = ..., metrics_accepted: _Optional[int] = ..., logs_accepted: _Optional[int] = ..., errors: _Optional[_Iterable[str]] = ...) -> None: ...
