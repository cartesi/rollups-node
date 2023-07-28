// (c) Cartesi and individual authors (see AUTHORS)
// SPDX-License-Identifier: Apache-2.0 (see LICENSE)

use crate::state_fold::Foldable;

use super::archive::Archive;

use std::any::{Any, TypeId};
use std::collections::HashMap;
use std::sync::Arc;

use tokio::sync::RwLock;

pub(crate) struct GlobalArchive {
    safety_margin: usize,
    archives: RwLock<HashMap<TypeId, Arc<dyn Any + Send + Sync + 'static>>>,
}

impl GlobalArchive {
    pub fn new(safety_margin: usize) -> Self {
        Self {
            safety_margin,
            archives: RwLock::new(HashMap::new()),
        }
    }

    pub(crate) async fn get_archive<F>(&self) -> Arc<Archive<F>>
    where
        F: Foldable + Send + Sync + 'static,
    {
        if let Some(archive) =
            self.archives.read().await.get(&TypeId::of::<Archive<F>>())
        {
            return archive.clone().downcast::<Archive<F>>().unwrap();
        }

        let new_archive = Arc::new(Archive::new(self.safety_margin));
        self.archives
            .write()
            .await
            .insert(TypeId::of::<Archive<F>>(), new_archive.clone());

        new_archive
    }
}

#[cfg(test)]
mod tests {
    use super::GlobalArchive;
    use crate::state_fold::test_utils::mocks::{IncrementFold, MockFold};
    use std::sync::Arc;

    #[tokio::test]
    async fn test_dyn_type() {
        let global_archive = GlobalArchive::new(42);
        assert_eq!(0, global_archive.archives.read().await.len());

        let archive1 = global_archive.get_archive::<IncrementFold>().await;
        assert_eq!(1, global_archive.archives.read().await.len());

        let archive2 = global_archive.get_archive::<IncrementFold>().await;
        assert_eq!(1, global_archive.archives.read().await.len());

        assert!(Arc::ptr_eq(&archive1, &archive2));

        let archive3 = global_archive.get_archive::<MockFold>().await;
        assert_eq!(2, global_archive.archives.read().await.len());

        let archive4 = global_archive.get_archive::<MockFold>().await;
        assert_eq!(2, global_archive.archives.read().await.len());

        assert!(Arc::ptr_eq(&archive3, &archive4));
    }
}
