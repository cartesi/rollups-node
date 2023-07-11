// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use base64::{engine::general_purpose::STANDARD as base64_engine, Engine as _};
use snafu::ResultExt;
use std::fmt::Debug;

use super::error::{
    DecodeBase64CursorSnafu, DecodeUTF8CursorSnafu, Error,
    MixedPaginationSnafu, PaginationCursorSnafu, PaginationLimitSnafu,
    ParseCursorSnafu,
};

const DEFAULT_PAGINATION_LIMIT: i32 = 1000;

macro_rules! ensure_cursor {
    ($arg: ident, $total_count: expr) => {{
        let cursor = Cursor::decode(&$arg)?;
        snafu::ensure!(
            cursor.offset >= 0 && cursor.offset < $total_count,
            PaginationCursorSnafu {
                arg: stringify!($arg),
            }
        );
        cursor.offset
    }};
}

macro_rules! ensure_limit {
    ($arg: ident) => {
        match $arg {
            Some(limit) => {
                snafu::ensure!(
                    limit >= 0,
                    PaginationLimitSnafu {
                        arg: stringify!($arg),
                    }
                );
                std::cmp::min(limit, DEFAULT_PAGINATION_LIMIT)
            }
            None => DEFAULT_PAGINATION_LIMIT,
        }
    };
}

#[derive(Debug, PartialEq)]
pub struct Pagination {
    total_count: i32,
    offset: i32,
    limit: i32,
}

impl Pagination {
    pub fn new(
        first: Option<i32>,
        last: Option<i32>,
        after: Option<String>,
        before: Option<String>,
        total_count: i32,
    ) -> Result<Self, Error> {
        let forward = first.is_some() || after.is_some();
        let backward = last.is_some() || before.is_some();
        snafu::ensure!(!forward || !backward, MixedPaginationSnafu);
        if backward {
            let before_offset = match before {
                Some(before) => ensure_cursor!(before, total_count),
                None => total_count,
            };
            let limit = ensure_limit!(last);
            if limit >= before_offset {
                Ok(Self {
                    total_count,
                    offset: 0,
                    limit: before_offset,
                })
            } else {
                Ok(Self {
                    total_count,
                    offset: before_offset - limit,
                    limit,
                })
            }
        } else {
            let offset = match after {
                Some(after) => ensure_cursor!(after, total_count) + 1,
                None => 0,
            };
            let limit = ensure_limit!(first);
            if offset + limit > total_count {
                Ok(Self {
                    total_count,
                    offset,
                    limit: total_count - offset,
                })
            } else {
                Ok(Self {
                    total_count,
                    offset,
                    limit,
                })
            }
        }
    }

    pub fn offset(&self) -> i32 {
        self.offset
    }

    pub fn limit(&self) -> i32 {
        self.limit
    }

    pub fn create_connection<T: Debug>(&self, nodes: Vec<T>) -> Connection<T> {
        let mut edges = vec![];
        for (i, node) in nodes.into_iter().enumerate() {
            let cursor = Cursor {
                offset: self.offset + i as i32,
            };
            edges.push(Edge { node, cursor });
        }
        let (start_cursor, has_previous_page) =
            if let Some(edge) = edges.first() {
                (Some(edge.cursor), edge.cursor.offset > 0)
            } else {
                (None, false)
            };
        let (end_cursor, has_next_page) = if let Some(edge) = edges.last() {
            (Some(edge.cursor), edge.cursor.offset < self.total_count - 1)
        } else {
            (None, false)
        };
        let page_info = PageInfo {
            start_cursor,
            end_cursor,
            has_next_page,
            has_previous_page,
        };
        Connection {
            total_count: self.total_count,
            edges,
            page_info,
        }
    }
}

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct Cursor {
    offset: i32,
}

impl Cursor {
    /// Encode cursor as base64
    pub fn encode(&self) -> String {
        base64_engine.encode(self.offset.to_string())
    }

    /// Decode cursor from base64 String
    pub fn decode(value: &str) -> Result<Cursor, Error> {
        let bytes = base64_engine
            .decode(value)
            .context(DecodeBase64CursorSnafu)?;
        let offset = std::str::from_utf8(&bytes)
            .context(DecodeUTF8CursorSnafu)?
            .parse::<i32>()
            .context(ParseCursorSnafu)?;
        Ok(Cursor { offset })
    }
}

#[derive(Debug, PartialEq)]
pub struct Connection<N: Debug> {
    pub total_count: i32,
    pub edges: Vec<Edge<N>>,
    pub page_info: PageInfo,
}

#[derive(Debug, PartialEq)]
pub struct Edge<N: Debug> {
    pub node: N,
    pub cursor: Cursor,
}

#[derive(Debug, PartialEq)]
pub struct PageInfo {
    pub start_cursor: Option<Cursor>,
    pub end_cursor: Option<Cursor>,
    pub has_next_page: bool,
    pub has_previous_page: bool,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn it_encodes_cursor() {
        assert_eq!(Cursor { offset: 0 }.encode(), "MA==");
        assert_eq!(Cursor { offset: 1 }.encode(), "MQ==");
        assert_eq!(Cursor { offset: 2 }.encode(), "Mg==");
        assert_eq!(Cursor { offset: 1000 }.encode(), "MTAwMA==");
        assert_eq!(Cursor { offset: i32::MAX }.encode(), "MjE0NzQ4MzY0Nw==");
    }

    #[test]
    fn it_decodes_cursor() {
        assert_eq!(Cursor::decode("MA==").unwrap(), Cursor { offset: 0 });
        assert_eq!(Cursor::decode("MQ==").unwrap(), Cursor { offset: 1 });
        assert_eq!(Cursor::decode("Mg==").unwrap(), Cursor { offset: 2 });
        assert_eq!(
            Cursor::decode("MTAwMA==").unwrap(),
            Cursor { offset: 1000 }
        );
        assert_eq!(
            Cursor::decode("MjE0NzQ4MzY0Nw==").unwrap(),
            Cursor { offset: i32::MAX }
        );
    }

    #[test]
    fn it_fails_to_decode_non_base64_cursor() {
        assert!(matches!(
            Cursor::decode("invalid").unwrap_err(),
            Error::DecodeBase64CursorError { .. }
        ))
    }

    #[test]
    fn it_fails_to_decode_invalid_string_cursor() {
        assert!(matches!(
            Cursor::decode("gA==").unwrap_err(),
            Error::DecodeUTF8CursorError { .. }
        ));
    }

    #[test]
    fn it_fails_to_decode_non_integer_cursor() {
        assert!(matches!(
            Cursor::decode("aW52YWxpZA==").unwrap_err(),
            Error::ParseCursorError { .. }
        ));
    }

    #[test]
    fn it_paginates_forward_by_default() {
        assert_eq!(
            Pagination::new(None, None, None, None, 1).unwrap(),
            Pagination {
                total_count: 1,
                offset: 0,
                limit: 1
            }
        );
        assert_eq!(
            Pagination::new(None, None, None, None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 0,
                limit: 10
            }
        );
        assert_eq!(
            Pagination::new(
                None,
                None,
                None,
                None,
                DEFAULT_PAGINATION_LIMIT * 10
            )
            .unwrap(),
            Pagination {
                total_count: DEFAULT_PAGINATION_LIMIT * 10,
                offset: 0,
                limit: DEFAULT_PAGINATION_LIMIT
            }
        );
    }

    #[test]
    fn it_paginates_with_zero_total_count() {
        assert_eq!(
            Pagination::new(Some(10), None, None, None, 0).unwrap(),
            Pagination {
                total_count: 0,
                offset: 0,
                limit: 0
            }
        );
        assert_eq!(
            Pagination::new(None, Some(10), None, None, 0).unwrap(),
            Pagination {
                total_count: 0,
                offset: 0,
                limit: 0
            }
        );
    }

    #[test]
    fn it_paginates_forward_with_bounded_limit() {
        assert_eq!(
            Pagination::new(Some(5), None, None, None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 0,
                limit: 5
            }
        );
        let cursor = Cursor { offset: 2 }.encode();
        assert_eq!(
            Pagination::new(Some(5), None, Some(cursor), None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 3,
                limit: 5
            }
        );
        let cursor = Cursor { offset: 4 }.encode();
        assert_eq!(
            Pagination::new(Some(5), None, Some(cursor), None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 5,
                limit: 5
            }
        );
    }

    #[test]
    fn it_paginates_forward_with_out_of_bounds_limit() {
        let cursor = Cursor { offset: 7 }.encode();
        assert_eq!(
            Pagination::new(Some(5), None, Some(cursor), None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 8,
                limit: 2
            }
        );
        let cursor = Cursor { offset: 9 }.encode();
        assert_eq!(
            Pagination::new(Some(5), None, Some(cursor), None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 10,
                limit: 0
            }
        );
    }

    #[test]
    fn it_paginates_backward_with_bounded_limit() {
        assert_eq!(
            Pagination::new(None, Some(5), None, None, 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 5,
                limit: 5
            }
        );
        let cursor = Cursor { offset: 7 }.encode();
        assert_eq!(
            Pagination::new(None, Some(5), None, Some(cursor), 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 2,
                limit: 5
            }
        );
        let cursor = Cursor { offset: 5 }.encode();
        assert_eq!(
            Pagination::new(None, Some(5), None, Some(cursor), 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 0,
                limit: 5
            }
        );
    }

    #[test]
    fn it_paginates_backward_with_out_of_bounds_limit() {
        let cursor = Cursor { offset: 3 }.encode();
        assert_eq!(
            Pagination::new(None, Some(5), None, Some(cursor), 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 0,
                limit: 3
            }
        );
        let cursor = Cursor { offset: 0 }.encode();
        assert_eq!(
            Pagination::new(None, Some(5), None, Some(cursor), 10).unwrap(),
            Pagination {
                total_count: 10,
                offset: 0,
                limit: 0
            }
        );
    }

    #[test]
    fn it_fails_to_paginate_when_mixing_backward_and_forward_args() {
        let cursor = Cursor { offset: 0 }.encode();
        assert!(matches!(
            Pagination::new(Some(1), Some(1), None, None, 1).unwrap_err(),
            Error::MixedPaginationError {}
        ));
        assert!(matches!(
            Pagination::new(None, Some(1), Some(cursor.clone()), None, 1)
                .unwrap_err(),
            Error::MixedPaginationError {}
        ));
        assert!(matches!(
            Pagination::new(Some(1), None, None, Some(cursor.clone()), 1)
                .unwrap_err(),
            Error::MixedPaginationError {}
        ));
        assert!(matches!(
            Pagination::new(
                None,
                None,
                Some(cursor.clone()),
                Some(cursor.clone()),
                1
            )
            .unwrap_err(),
            Error::MixedPaginationError {}
        ));
    }

    #[test]
    fn it_fails_to_paginate_when_limit_is_negative() {
        assert!(matches!(
            Pagination::new(Some(-1), None, None, None, 10).unwrap_err(),
            Error::PaginationLimitError { arg } if arg == "first"
        ));
        assert!(matches!(
            Pagination::new(None, Some(-1), None, None, 10).unwrap_err(),
            Error::PaginationLimitError { arg } if arg == "last"
        ));
    }

    #[test]
    fn it_fails_to_paginate_with_invalid_cursor() {
        assert!(matches!(
            Pagination::new(None, None, Some("invalid".to_owned()), None, 10)
                .unwrap_err(),
            Error::DecodeBase64CursorError { .. }
        ));
        assert!(matches!(
            Pagination::new(None, None, None, Some("invalid".to_owned()), 10)
                .unwrap_err(),
            Error::DecodeBase64CursorError { .. }
        ));
    }

    #[test]
    fn it_fails_to_paginate_with_cursor_out_of_range() {
        let cursor = Cursor { offset: 10 }.encode();
        assert!(matches!(
            Pagination::new(None, None, Some(cursor.clone()), None, 10)
                .unwrap_err(),
            Error::PaginationCursorError { arg } if arg == "after"
        ));
        assert!(matches!(
            Pagination::new(None, None, None, Some(cursor.clone()), 10)
                .unwrap_err(),
            Error::PaginationCursorError { arg } if arg == "before"
        ));
    }

    #[test]
    fn it_creates_connection_without_nodes() {
        let pagination = Pagination {
            total_count: 3,
            offset: 0,
            limit: 0,
        };
        let connection = pagination.create_connection::<String>(vec![]);
        assert_eq!(
            connection,
            Connection {
                total_count: 3,
                edges: vec![],
                page_info: PageInfo {
                    start_cursor: None,
                    end_cursor: None,
                    has_next_page: false,
                    has_previous_page: false,
                }
            }
        );
    }

    #[test]
    fn it_creates_connection_with_all_nodes() {
        let pagination = Pagination {
            total_count: 3,
            offset: 0,
            limit: 3,
        };
        let connection = pagination.create_connection::<String>(vec![
            "0".to_owned(),
            "1".to_owned(),
            "2".to_owned(),
        ]);
        assert_eq!(
            connection,
            Connection {
                total_count: 3,
                edges: vec![
                    Edge {
                        node: "0".to_owned(),
                        cursor: Cursor { offset: 0 },
                    },
                    Edge {
                        node: "1".to_owned(),
                        cursor: Cursor { offset: 1 },
                    },
                    Edge {
                        node: "2".to_owned(),
                        cursor: Cursor { offset: 2 },
                    },
                ],
                page_info: PageInfo {
                    start_cursor: Some(Cursor { offset: 0 }),
                    end_cursor: Some(Cursor { offset: 2 }),
                    has_next_page: false,
                    has_previous_page: false,
                }
            }
        );
    }

    #[test]
    fn it_creates_connection_on_first_page() {
        let pagination = Pagination {
            total_count: 3,
            offset: 0,
            limit: 2,
        };
        let connection = pagination
            .create_connection::<String>(vec!["0".to_owned(), "1".to_owned()]);
        assert_eq!(
            connection,
            Connection {
                total_count: 3,
                edges: vec![
                    Edge {
                        node: "0".to_owned(),
                        cursor: Cursor { offset: 0 },
                    },
                    Edge {
                        node: "1".to_owned(),
                        cursor: Cursor { offset: 1 },
                    },
                ],
                page_info: PageInfo {
                    start_cursor: Some(Cursor { offset: 0 }),
                    end_cursor: Some(Cursor { offset: 1 }),
                    has_next_page: true,
                    has_previous_page: false,
                }
            }
        );
    }

    #[test]
    fn it_creates_connection_on_last_page() {
        let pagination = Pagination {
            total_count: 3,
            offset: 1,
            limit: 2,
        };
        let connection = pagination
            .create_connection::<String>(vec!["1".to_owned(), "2".to_owned()]);
        assert_eq!(
            connection,
            Connection {
                total_count: 3,
                edges: vec![
                    Edge {
                        node: "1".to_owned(),
                        cursor: Cursor { offset: 1 },
                    },
                    Edge {
                        node: "2".to_owned(),
                        cursor: Cursor { offset: 2 },
                    },
                ],
                page_info: PageInfo {
                    start_cursor: Some(Cursor { offset: 1 }),
                    end_cursor: Some(Cursor { offset: 2 }),
                    has_next_page: false,
                    has_previous_page: true,
                }
            }
        );
    }

    #[test]
    fn it_creates_connection_on_middle_page() {
        let pagination = Pagination {
            total_count: 3,
            offset: 1,
            limit: 1,
        };
        let connection =
            pagination.create_connection::<String>(vec!["1".to_owned()]);
        assert_eq!(
            connection,
            Connection {
                total_count: 3,
                edges: vec![Edge {
                    node: "1".to_owned(),
                    cursor: Cursor { offset: 1 },
                },],
                page_info: PageInfo {
                    start_cursor: Some(Cursor { offset: 1 }),
                    end_cursor: Some(Cursor { offset: 1 }),
                    has_next_page: true,
                    has_previous_page: true,
                }
            }
        );
    }
}
