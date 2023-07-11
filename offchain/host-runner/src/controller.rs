// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use async_trait::async_trait;
use snafu::Snafu;
use std::time::Duration;
use tokio::sync::{mpsc, oneshot};

use crate::model::*;

const MPSC_BUFFER_SIZE: usize = 1000;

/// State-machine that controls the rollup state.
#[derive(Clone, Debug)]
pub struct Controller {
    advance_tx: mpsc::Sender<SyncAdvanceStateRequest>,
    inspect_tx: mpsc::Sender<SyncInspectResult>,
    finish_tx: mpsc::Sender<SyncFinishRequest>,
    voucher_tx: mpsc::Sender<SyncVoucherRequest>,
    notice_tx: mpsc::Sender<SyncNoticeRequest>,
    report_tx: mpsc::Sender<SyncReportRequest>,
    exception_tx: mpsc::Sender<SyncExceptionRequest>,
    shutdown_tx: mpsc::Sender<SyncShutdownRequest>,
}

impl Controller {
    pub fn new(finish_timeout: Duration) -> Self {
        let (advance_tx, advance_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (inspect_tx, inspect_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (voucher_tx, voucher_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (notice_tx, notice_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (report_tx, report_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (finish_tx, finish_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (exception_tx, exception_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let (shutdown_tx, shutdown_rx) = mpsc::channel(MPSC_BUFFER_SIZE);
        let data = SharedStateData {
            advance_rx,
            inspect_rx,
            voucher_rx,
            notice_rx,
            report_rx,
            finish_rx,
            exception_rx,
            shutdown_rx,
            finish_timeout,
        };
        let service = Service::new(data);
        tokio::spawn(service.run());
        Self {
            advance_tx,
            inspect_tx,
            voucher_tx,
            notice_tx,
            report_tx,
            finish_tx,
            exception_tx,
            shutdown_tx,
        }
    }

    pub async fn advance(
        &self,
        request: AdvanceStateRequest,
    ) -> oneshot::Receiver<AdvanceResult> {
        SyncRequest::send(&self.advance_tx, request).await
    }

    pub async fn inspect(
        &self,
        request: InspectStateRequest,
    ) -> oneshot::Receiver<InspectResult> {
        SyncRequest::send(&self.inspect_tx, request).await
    }

    pub async fn finish(
        &self,
        status: FinishStatus,
    ) -> oneshot::Receiver<Result<RollupRequest, ControllerError>> {
        SyncRequest::send(&self.finish_tx, status).await
    }

    pub async fn insert_voucher(
        &self,
        voucher: Voucher,
    ) -> oneshot::Receiver<Result<usize, ControllerError>> {
        SyncRequest::send(&self.voucher_tx, voucher).await
    }

    pub async fn insert_notice(
        &self,
        notice: Notice,
    ) -> oneshot::Receiver<Result<usize, ControllerError>> {
        SyncRequest::send(&self.notice_tx, notice).await
    }

    pub async fn insert_report(
        &self,
        report: Report,
    ) -> oneshot::Receiver<Result<(), ControllerError>> {
        SyncRequest::send(&self.report_tx, report).await
    }

    pub async fn notify_exception(
        &self,
        exception: RollupException,
    ) -> oneshot::Receiver<Result<(), ControllerError>> {
        SyncRequest::send(&self.exception_tx, exception).await
    }

    pub async fn shutdown(&self) -> oneshot::Receiver<()> {
        SyncRequest::send(&self.shutdown_tx, ()).await
    }
}

#[derive(Debug, PartialEq, Snafu)]
pub enum ControllerError {
    #[snafu(display("no rollup request available"))]
    FetchRequestTimeout,
    #[snafu(display(
        "invalid request {} in {} state",
        request_name,
        state_name
    ))]
    InvalidRequest {
        request_name: String,
        state_name: String,
    },
}

struct Service {
    state: Box<dyn State>,
}

impl Service {
    fn new(data: SharedStateData) -> Self {
        Self {
            state: IdleState::new(data),
        }
    }

    async fn run(mut self) {
        loop {
            if let Some(state) = self.state.process().await {
                self.state = state;
            } else {
                tracing::info!("controller service terminated successfully");
                break;
            }
        }
    }

    fn handle_invalid<T, U>(
        request: SyncRequest<T, Result<U, ControllerError>>,
        state: Box<dyn State>,
        request_name: &str,
    ) -> Option<Box<dyn State>>
    where
        T: std::fmt::Debug + Send + Sync,
        U: std::fmt::Debug + Send + Sync,
    {
        let err = ControllerError::InvalidRequest {
            state_name: state.name(),
            request_name: request_name.into(),
        };
        tracing::warn!("{}", err.to_string());
        let (_, response_tx) = request.into_inner();
        send_response(response_tx, Err(err));
        Some(state)
    }

    fn shutdown(request: SyncShutdownRequest) -> Option<Box<dyn State>> {
        tracing::info!("processing shutdown request");
        request.process(|_| ());
        None
    }
}

struct SyncRequest<T, U>
where
    T: Send + Sync,
    U: Send + Sync,
{
    request: T,
    response_tx: oneshot::Sender<U>,
}

impl<T, U> SyncRequest<T, U>
where
    T: std::fmt::Debug + Send + Sync,
    U: std::fmt::Debug + Send + Sync,
{
    async fn send(tx: &mpsc::Sender<Self>, request: T) -> oneshot::Receiver<U> {
        let (response_tx, response_rx) = oneshot::channel();
        if let Err(e) = tx
            .send(SyncRequest {
                request,
                response_tx,
            })
            .await
        {
            tracing::error!("failed to send request ({})", e)
        }
        response_rx
    }

    fn into_inner(self) -> (T, oneshot::Sender<U>) {
        (self.request, self.response_tx)
    }

    fn process<F>(self, f: F)
    where
        F: FnOnce(T) -> U,
    {
        let response = f(self.request);
        send_response(self.response_tx, response);
    }
}

fn send_response<U>(tx: oneshot::Sender<U>, response: U)
where
    U: std::fmt::Debug + Send + Sync,
{
    if tx.send(response).is_err() {
        tracing::warn!("failed to send response (channel dropped)");
    }
}

impl<T, U> std::fmt::Debug for SyncRequest<T, U>
where
    T: std::fmt::Debug + Send + Sync,
    U: std::fmt::Debug + Send + Sync,
{
    fn fmt(&self, f: &mut std::fmt::Formatter) -> Result<(), std::fmt::Error> {
        write!(f, "{:?}", self.request)
    }
}

type SyncAdvanceStateRequest = SyncRequest<AdvanceStateRequest, AdvanceResult>;
type SyncInspectResult = SyncRequest<InspectStateRequest, InspectResult>;
type SyncFinishRequest =
    SyncRequest<FinishStatus, Result<RollupRequest, ControllerError>>;
type SyncVoucherRequest = SyncRequest<Voucher, Result<usize, ControllerError>>;
type SyncNoticeRequest = SyncRequest<Notice, Result<usize, ControllerError>>;
type SyncReportRequest = SyncRequest<Report, Result<(), ControllerError>>;
type SyncExceptionRequest =
    SyncRequest<RollupException, Result<(), ControllerError>>;
type SyncShutdownRequest = SyncRequest<(), ()>;

struct SharedStateData {
    advance_rx: mpsc::Receiver<SyncAdvanceStateRequest>,
    inspect_rx: mpsc::Receiver<SyncInspectResult>,
    finish_rx: mpsc::Receiver<SyncFinishRequest>,
    voucher_rx: mpsc::Receiver<SyncVoucherRequest>,
    notice_rx: mpsc::Receiver<SyncNoticeRequest>,
    report_rx: mpsc::Receiver<SyncReportRequest>,
    exception_rx: mpsc::Receiver<SyncExceptionRequest>,
    shutdown_rx: mpsc::Receiver<SyncShutdownRequest>,
    finish_timeout: Duration,
}

/// OOP state design-pattern
#[async_trait]
trait State: Send + Sync {
    async fn process(self: Box<Self>) -> Option<Box<dyn State>>;
    fn name(&self) -> String;
}

/// The controller waits for finish request from the DApp
struct IdleState {
    data: SharedStateData,
}

impl IdleState {
    fn new(data: SharedStateData) -> Box<dyn State> {
        Box::new(Self { data })
    }
}

#[async_trait]
impl State for IdleState {
    async fn process(mut self: Box<Self>) -> Option<Box<dyn State>> {
        tokio::select! {
            biased;
            Some(request) = self.data.finish_rx.recv() => {
                tracing::debug!("received finish request; changing state to fetch request");
                tracing::debug!("request: {:?}", request);
                let (_, response_tx) = request.into_inner();
                Some(FetchRequestState::new(self.data, response_tx))
            }
            Some(request) = self.data.voucher_rx.recv() => {
                Service::handle_invalid(request, self, "voucher")
            }
            Some(request) = self.data.notice_rx.recv() => {
                Service::handle_invalid(request, self, "notice")
            }
            Some(request) = self.data.report_rx.recv() => {
                Service::handle_invalid(request, self, "report")
            }
            Some(request) = self.data.exception_rx.recv() => {
                Service::handle_invalid(request, self, "exception")
            }
            Some(request) = self.data.shutdown_rx.recv() => {
                Service::shutdown(request)
            }
        }
    }

    fn name(&self) -> String {
        "idle".into()
    }
}

/// The controller waits for either an inspect of an advance request from the gRPC service
struct FetchRequestState {
    data: SharedStateData,
    finish_response_tx: oneshot::Sender<Result<RollupRequest, ControllerError>>,
}

impl FetchRequestState {
    fn new(
        data: SharedStateData,
        finish_response_tx: oneshot::Sender<
            Result<RollupRequest, ControllerError>,
        >,
    ) -> Box<dyn State> {
        Box::new(Self {
            data,
            finish_response_tx,
        })
    }
}

#[async_trait]
impl State for FetchRequestState {
    async fn process(mut self: Box<Self>) -> Option<Box<dyn State>> {
        tokio::select! {
            biased;
            _ = tokio::time::sleep(self.data.finish_timeout) => {
                tracing::debug!("fetch request timed out; setting state to idle");
                let timeout_err = ControllerError::FetchRequestTimeout;
                send_response(self.finish_response_tx, Err(timeout_err));
                Some(IdleState::new(self.data))
            }
            Some(request) = self.data.inspect_rx.recv() => {
                tracing::debug!("received inspect request; setting state to inspect");
                tracing::debug!("request: {:?}", request);
                let (inspect_request, inspect_response_tx) = request.into_inner();
                let rollup_request = RollupRequest::InspectState(inspect_request);
                send_response(self.finish_response_tx, Ok(rollup_request));
                Some(InspectState::new(self.data, inspect_response_tx))
            }
            Some(request) = self.data.advance_rx.recv() => {
                tracing::debug!("received advance request; setting state to advance");
                tracing::debug!("request: {:?}", request);
                let (advance_request, advance_response_tx) = request.into_inner();
                let rollup_request = RollupRequest::AdvanceState(advance_request);
                send_response(self.finish_response_tx, Ok(rollup_request));
                Some(AdvanceState::new(self.data, advance_response_tx))
            }
            Some(request) = self.data.finish_rx.recv() => {
                tracing::debug!("received finish request; terminating previous finish request");
                tracing::debug!("request: {:?}", request);
                let timeout_err = ControllerError::FetchRequestTimeout;
                send_response(self.finish_response_tx, Err(timeout_err));
                let (_, response_tx) = request.into_inner();
                Some(FetchRequestState::new(self.data, response_tx))
            }
            Some(request) = self.data.voucher_rx.recv() => {
                Service::handle_invalid(request, self, "voucher")
            }
            Some(request) = self.data.notice_rx.recv() => {
                Service::handle_invalid(request, self, "notice")
            }
            Some(request) = self.data.report_rx.recv() => {
                Service::handle_invalid(request, self, "report")
            }
            Some(request) = self.data.exception_rx.recv() => {
                Service::handle_invalid(request, self, "exception")
            }
            Some(request) = self.data.shutdown_rx.recv() => {
                Service::shutdown(request)
            }
        }
    }

    fn name(&self) -> String {
        "fetch request".into()
    }
}

/// The controller wait for reports, exception, and finish
struct InspectState {
    data: SharedStateData,
    inspect_response_tx: oneshot::Sender<InspectResult>,
    reports: Vec<Report>,
}

impl InspectState {
    fn new(
        data: SharedStateData,
        inspect_response_tx: oneshot::Sender<InspectResult>,
    ) -> Box<dyn State> {
        Box::new(Self {
            data,
            inspect_response_tx,
            reports: vec![],
        })
    }
}

#[async_trait]
impl State for InspectState {
    async fn process(mut self: Box<Self>) -> Option<Box<dyn State>> {
        tokio::select! {
            biased;
            Some(request) = self.data.finish_rx.recv() => {
                tracing::debug!("received finish request; changing state to fetch request");
                tracing::debug!("request: {:?}", request);
                let (status, response_tx) = request.into_inner();
                let result = match status {
                    FinishStatus::Accept => InspectResult::accepted(self.reports),
                    FinishStatus::Reject => InspectResult::rejected(self.reports),
                };
                send_response(self.inspect_response_tx, result);
                Some(FetchRequestState::new(self.data, response_tx))
            }
            Some(request) = self.data.report_rx.recv() => {
                tracing::debug!("received report request");
                tracing::debug!("request: {:?}", request);
                request.process(|report| {
                    self.reports.push(report);
                    Ok(())
                });
                Some(self)
            }
            Some(request) = self.data.exception_rx.recv() => {
                tracing::debug!("received exception request; setting state to idle");
                tracing::debug!("request: {:?}", request);
                let (exception, exception_response_tx) = request.into_inner();
                let result = InspectResult::exception(self.reports, exception);
                send_response(self.inspect_response_tx, result);
                send_response(exception_response_tx, Ok(()));
                Some(IdleState::new(self.data))
            }
            Some(request) = self.data.voucher_rx.recv() => {
                Service::handle_invalid(request, self, "voucher")
            }
            Some(request) = self.data.notice_rx.recv() => {
                Service::handle_invalid(request, self, "notice")
            }
            Some(request) = self.data.shutdown_rx.recv() => {
                Service::shutdown(request)
            }
        }
    }

    fn name(&self) -> String {
        "inspect".into()
    }
}

/// The controller waits for vouchers, notices, reports, exception, and finish
struct AdvanceState {
    data: SharedStateData,
    advance_response_tx: oneshot::Sender<AdvanceResult>,
    vouchers: Vec<Voucher>,
    notices: Vec<Notice>,
    reports: Vec<Report>,
}

impl AdvanceState {
    fn new(
        data: SharedStateData,
        advance_response_tx: oneshot::Sender<AdvanceResult>,
    ) -> Box<dyn State> {
        Box::new(Self {
            data,
            advance_response_tx,
            vouchers: vec![],
            notices: vec![],
            reports: vec![],
        })
    }
}

#[async_trait]
impl State for AdvanceState {
    async fn process(mut self: Box<Self>) -> Option<Box<dyn State>> {
        tokio::select! {
            biased;
            Some(request) = self.data.finish_rx.recv() => {
                tracing::debug!("received finish request; changing state to fetch request");
                tracing::debug!("request: {:?}", request);
                let (status, response_tx) = request.into_inner();
                let result = match status {
                    FinishStatus::Accept => {
                        AdvanceResult::accepted(
                            self.vouchers,
                            self.notices,
                            self.reports,
                        )
                    },
                    FinishStatus::Reject => {
                        AdvanceResult::rejected(
                            self.reports,
                        )
                    },
                };
                send_response(self.advance_response_tx, result);
                Some(FetchRequestState::new(self.data, response_tx))
            }
            Some(request) = self.data.voucher_rx.recv() => {
                tracing::debug!("received voucher request");
                tracing::debug!("request: {:?}", request);
                request.process(|voucher| {
                    self.vouchers.push(voucher);
                    Ok(self.vouchers.len() - 1)
                });
                Some(self)
            }
            Some(request) = self.data.notice_rx.recv() => {
                tracing::debug!("received notice request");
                tracing::debug!("request: {:?}", request);
                request.process(|notice| {
                    self.notices.push(notice);
                    Ok(self.notices.len() - 1)
                });
                Some(self)
            }
            Some(request) = self.data.report_rx.recv() => {
                tracing::debug!("received report request");
                tracing::debug!("request: {:?}", request);
                request.process(|report| {
                    self.reports.push(report);
                    Ok(())
                });
                Some(self)
            }
            Some(request) = self.data.exception_rx.recv() => {
                tracing::debug!("received exception request; setting state to idle");
                tracing::debug!("request: {:?}", request);
                let (exception, exception_response_tx) = request.into_inner();
                let result = AdvanceResult::exception(
                    exception,
                    self.reports,
                );
                send_response(self.advance_response_tx, result);
                send_response(exception_response_tx, Ok(()));
                Some(IdleState::new(self.data))
            }
            Some(request) = self.data.shutdown_rx.recv() => {
                Service::shutdown(request)
            }
        }
    }

    fn name(&self) -> String {
        "advance".into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const TEST_FINISH_TIMEOUT: Duration = Duration::from_millis(100);

    fn setup() -> Controller {
        Controller::new(TEST_FINISH_TIMEOUT)
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_rejects_invalid_requests_in_idle_state() {
        let controller = setup();
        let rx = controller.insert_voucher(mock_voucher()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("voucher"),
                state_name: String::from("idle")
            }
        );
        let rx = controller.insert_notice(mock_notice()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("notice"),
                state_name: String::from("idle")
            }
        );
        let rx = controller.insert_report(mock_report()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("report"),
                state_name: String::from("idle")
            }
        );
        let rx = controller.notify_exception(mock_exception()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("exception"),
                state_name: String::from("idle")
            }
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_handles_multiple_finish_requests_at_the_same_time() {
        let controller = setup();
        let mut handlers = vec![];
        const N: usize = 3;
        for _ in 0..N {
            let handler = {
                let controller = controller.clone();
                tokio::spawn(async move {
                    controller.finish(FinishStatus::Accept).await
                })
            };
            handlers.push(handler);
        }
        for handler in handlers {
            let rx = handler.await.unwrap();
            let timeout_err = rx.await.unwrap().unwrap_err();
            assert_eq!(timeout_err, ControllerError::FetchRequestTimeout);
        }
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_rejects_invalid_requests_in_fetch_request_state() {
        let controller = setup();
        // Set state to fetch request by calling finish once in another thread
        let _ = controller.finish(FinishStatus::Accept).await;
        let rx = controller.insert_voucher(mock_voucher()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("voucher"),
                state_name: String::from("fetch request")
            }
        );
        let rx = controller.insert_notice(mock_notice()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("notice"),
                state_name: String::from("fetch request")
            }
        );
        let rx = controller.insert_report(mock_report()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("report"),
                state_name: String::from("fetch request")
            }
        );
        let rx = controller.notify_exception(mock_exception()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("exception"),
                state_name: String::from("fetch request")
            }
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_rejects_invalid_requests_during_inspect() {
        let controller = setup();
        let _ = controller.inspect(mock_inspect_request()).await;
        let _ = controller
            .finish(FinishStatus::Accept)
            .await
            .await
            .unwrap()
            .unwrap();
        let rx = controller.insert_voucher(mock_voucher()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("voucher"),
                state_name: String::from("inspect")
            }
        );
        let rx = controller.insert_notice(mock_notice()).await;
        assert_eq!(
            rx.await.unwrap().unwrap_err(),
            ControllerError::InvalidRequest {
                request_name: String::from("notice"),
                state_name: String::from("inspect")
            }
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_advances_state_before_finish() {
        let controller = setup();
        // Send advance request
        let advance_request = mock_advance_request();
        let advance_rx = controller.advance(advance_request.clone()).await;
        // Send first finish request
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let rollup_request = finish_rx.await.unwrap().unwrap();
        assert_eq!(
            rollup_request,
            RollupRequest::AdvanceState(advance_request)
        );
        // Send second finish request
        let _ = controller.finish(FinishStatus::Accept).await;
        // Obtain result from advance request
        let advance_result = advance_rx.await.unwrap();
        assert_eq!(
            advance_result,
            AdvanceResult::accepted(vec![], vec![], vec![])
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_advances_state_after_finish() {
        let controller = setup();
        // Send first finish request
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        // Send advance request
        let advance_request = mock_advance_request();
        let advance_rx = controller.advance(advance_request.clone()).await;
        // Receive first finish result
        let rollup_request = finish_rx.await.unwrap().unwrap();
        assert_eq!(
            rollup_request,
            RollupRequest::AdvanceState(advance_request)
        );
        // Send second finish request
        let _ = controller.finish(FinishStatus::Accept).await;
        // Obtain result from advance request
        let advance_result = advance_rx.await.unwrap();
        assert_eq!(
            advance_result,
            AdvanceResult::accepted(vec![], vec![], vec![])
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_advances_state_after_previous_advance() {
        let controller = setup();
        const N: usize = 3;
        let mut advance_requests = std::collections::VecDeque::new();
        let mut advance_rxs = std::collections::VecDeque::new();
        // Send several advance requests before starting
        for _ in 0..N {
            let request = mock_advance_request();
            advance_requests.push_back(request.clone());
            let rx = controller.advance(request).await;
            advance_rxs.push_back(rx);
        }
        // Send first finish
        let mut finish_rx = controller.finish(FinishStatus::Accept).await;
        // Process each advance request
        while !advance_requests.is_empty() {
            let rollup_request = finish_rx.await.unwrap().unwrap();
            let expected_request = advance_requests.pop_front().unwrap();
            assert_eq!(
                rollup_request,
                RollupRequest::AdvanceState(expected_request)
            );
            finish_rx = controller.finish(FinishStatus::Accept).await;
            let _ = advance_rxs.pop_front().unwrap().await.unwrap();
        }
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_prioritizes_inspect_over_advance_requests() {
        let controller = setup();
        // Before first finish, send first an advance request and an inspect request
        let _ = controller.advance(mock_advance_request()).await;
        let _ = controller.inspect(mock_inspect_request()).await;
        // The received request should be the inspect state
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let rollup_request = finish_rx.await.unwrap().unwrap();
        assert!(matches!(rollup_request, RollupRequest::InspectState(_)));
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_times_out_during_fetch_request() {
        let controller = setup();
        // Send first finish request without sending a rollup request
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let timeout_err = finish_rx.await.unwrap().unwrap_err();
        assert_eq!(timeout_err, ControllerError::FetchRequestTimeout);
        // Send an advance request that should not timeout
        let advance_request = mock_advance_request();
        let _ = controller.advance(advance_request.clone()).await;
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let rollup_request = finish_rx.await.unwrap().unwrap();
        assert_eq!(
            rollup_request,
            RollupRequest::AdvanceState(advance_request)
        );
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_sends_vouchers_notices_and_reports_during_advance() {
        let controller = setup();
        // Set state to advance
        let advance_rx = controller.advance(mock_advance_request()).await;
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let _ = finish_rx.await.unwrap().unwrap();
        // Insert voucher
        let voucher = mock_voucher();
        let voucher_rx = controller.insert_voucher(voucher.clone()).await;
        let voucher_id = voucher_rx.await.unwrap().unwrap();
        assert_eq!(voucher_id, 0);
        // Insert notice
        let notice = mock_notice();
        let notice_rx = controller.insert_notice(notice.clone()).await;
        let notice_id = notice_rx.await.unwrap().unwrap();
        assert_eq!(notice_id, 0);
        // Insert report
        let report = mock_report();
        let report_rx = controller.insert_report(report.clone()).await;
        report_rx.await.unwrap().unwrap();
        // Finalize the current advance state
        let _ = controller.finish(FinishStatus::Reject).await;
        // Obtain the advance result
        let result = advance_rx.await.unwrap();
        assert_eq!(result, AdvanceResult::rejected(vec![report]));
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_sends_reports_during_inspect() {
        let controller = setup();
        // Set state to inspect
        let inspect_rx = controller.inspect(mock_inspect_request()).await;
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let _ = finish_rx.await.unwrap().unwrap();
        // Insert report
        let report = mock_report();
        let report_rx = controller.insert_report(report.clone()).await;
        report_rx.await.unwrap().unwrap();
        // Finalize the current advance state
        let _ = controller.finish(FinishStatus::Accept).await;
        // Obtain the inspect result
        let result = inspect_rx.await.unwrap();
        assert_eq!(result, InspectResult::accepted(vec![report]));
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_handles_exception_during_advance() {
        let controller = setup();
        // Set state to advance
        let advance_rx = controller.advance(mock_advance_request()).await;
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let _ = finish_rx.await.unwrap().unwrap();
        // Send rollup exception
        let exception = mock_exception();
        let exception_rx = controller.notify_exception(exception.clone()).await;
        exception_rx.await.unwrap().unwrap();
        let advance_result = advance_rx.await.unwrap();
        assert_eq!(advance_result, AdvanceResult::exception(exception, vec![]));
        controller.shutdown().await;
    }

    #[tokio::test]
    #[tracing_test::traced_test]
    async fn test_it_handles_exception_during_inspect() {
        let controller = setup();
        // Set state to inspect
        let inspect_rx = controller.inspect(mock_inspect_request()).await;
        let finish_rx = controller.finish(FinishStatus::Accept).await;
        let _ = finish_rx.await.unwrap().unwrap();
        // Send rollup exception
        let exception = mock_exception();
        let exception_rx = controller.notify_exception(exception.clone()).await;
        exception_rx.await.unwrap().unwrap();
        let result = inspect_rx.await.unwrap();
        assert_eq!(result, InspectResult::exception(vec![], exception));
        controller.shutdown().await;
    }

    fn mock_voucher() -> Voucher {
        Voucher::new(rand::random(), rand::random::<[u8; 32]>().into())
    }

    fn mock_notice() -> Notice {
        Notice::new(rand::random::<[u8; 32]>().into())
    }

    fn mock_report() -> Report {
        Report {
            payload: rand::random::<[u8; 32]>().into(),
        }
    }

    fn mock_exception() -> RollupException {
        RollupException {
            payload: rand::random::<[u8; 32]>().into(),
        }
    }

    fn mock_advance_request() -> AdvanceStateRequest {
        AdvanceStateRequest {
            metadata: AdvanceMetadata {
                msg_sender: rand::random(),
                epoch_index: rand::random(),
                input_index: rand::random(),
                block_number: rand::random(),
                timestamp: rand::random(),
            },
            payload: rand::random::<[u8; 32]>().into(),
        }
    }

    fn mock_inspect_request() -> InspectStateRequest {
        InspectStateRequest {
            payload: rand::random::<[u8; 32]>().into(),
        }
    }
}
