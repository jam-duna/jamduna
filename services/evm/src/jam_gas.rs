//! JAM-native gas model implementation
//!
//! This module provides a completely independent gas model that bypasses the vendor EVM gasometer
//! entirely. All EVM opcodes cost 0 gas, and only JAM host function measurements are used for
//! resource accounting.

use alloc::{format, vec::Vec};
use evm::GasMutState;
use evm::interpreter::{ExitError, ExitException, runtime::GasState};
use evm::standard::{Config, State};
use primitive_types::{H160, H256, U256};
use utils::functions::{log_debug, log_error};
use utils::host_functions::gas;

/// JAM-native gas state that completely bypasses vendor gasometer
///
/// This wrapper makes all EVM opcodes cost 0 gas and uses only JAM host function
/// measurements for actual resource consumption tracking.
pub struct JAMGasState<'config> {
    /// Wrapped EVM state (vendor gasometer ignored)
    inner: State<'config>,

    /// JAM host gas limit for this transaction
    jam_gas_limit: u64,

    /// JAM host gas consumed at transaction start
    jam_gas_baseline: u64,

    /// Current JAM host gas consumption
    jam_gas_used: u64,

    /// Debug tracking of gas consumption
    debug_enabled: bool,
}

impl<'config> JAMGasState<'config> {
    /// Create new JAM gas state from standard EVM state
    pub fn new(inner: State<'config>, jam_gas_limit: u64, debug_enabled: bool) -> Self {
        let jam_gas_baseline = unsafe { gas() };

        if debug_enabled {
            log_debug(&format!(
                "JAMGasState: initialized with limit={}, baseline={}",
                jam_gas_limit, jam_gas_baseline
            ));
        }

        Self {
            inner,
            jam_gas_limit,
            jam_gas_baseline,
            jam_gas_used: 0,
            debug_enabled,
        }
    }

    /// Calculate current JAM host gas consumption
    fn current_jam_gas_used(&self) -> u64 {
        let current_host_gas = unsafe { gas() };
        let consumed = self.jam_gas_baseline.saturating_sub(current_host_gas);

        if self.debug_enabled && consumed != self.jam_gas_used {
            log_debug(&format!(
                "JAMGasState: gas consumption update baseline={} current={} consumed={}",
                self.jam_gas_baseline, current_host_gas, consumed
            ));
        }

        consumed
    }
}

// Implement GasState trait - returns JAM gas, not vendor gas
impl<'config> GasState for JAMGasState<'config> {
    fn gas(&self) -> U256 {
        let used = self.current_jam_gas_used();
        let remaining = self.jam_gas_limit.saturating_sub(used);
        U256::from(remaining)
    }
}

// Implement GasMutState trait - records JAM gas, ignores vendor gas
impl<'config> GasMutState for JAMGasState<'config> {
    fn record_gas(&mut self, vendor_gas: U256) -> Result<(), ExitError> {
        // ZERO-COST OPCODES: Completely ignore vendor gas parameter
        // All EVM opcodes cost 0 - only JAM host function measures real consumption

        if self.debug_enabled && vendor_gas > U256::zero() {
            log_debug(&format!(
                "JAMGasState: ignoring vendor gas {} (zero-cost opcodes)",
                vendor_gas
            ));
        }

        let current_used = self.current_jam_gas_used();
        self.jam_gas_used = current_used;

        if current_used > self.jam_gas_limit {
            if self.debug_enabled {
                log_error(&format!(
                    "JAMGasState: OutOfGas detected - used {} > limit {}",
                    current_used, self.jam_gas_limit
                ));
            }
            Err(ExitException::OutOfGas.into())
        } else {
            self.jam_gas_used = current_used;
            Ok(()) // Always succeed for vendor gas - opcodes are free, only host consumption matters
        }
    }
}

// Delegate all other trait implementations to inner state
impl<'config> AsRef<evm::interpreter::runtime::RuntimeState> for JAMGasState<'config> {
    fn as_ref(&self) -> &evm::interpreter::runtime::RuntimeState {
        self.inner.as_ref()
    }
}

impl<'config> AsMut<evm::interpreter::runtime::RuntimeState> for JAMGasState<'config> {
    fn as_mut(&mut self) -> &mut evm::interpreter::runtime::RuntimeState {
        self.inner.as_mut()
    }
}

impl<'config> AsRef<Config> for JAMGasState<'config> {
    fn as_ref(&self) -> &Config {
        self.inner.as_ref()
    }
}

impl<'config> AsRef<evm::interpreter::runtime::RuntimeConfig> for JAMGasState<'config> {
    fn as_ref(&self) -> &evm::interpreter::runtime::RuntimeConfig {
        self.inner.as_ref()
    }
}

// For InvokerState compatibility - delegate to inner state
impl<'config> evm::standard::InvokerState for JAMGasState<'config> {
    type TransactArgs = evm::standard::TransactArgs<'config>;

    fn new_transact_call(
        runtime: evm::interpreter::runtime::RuntimeState,
        gas_limit: U256,
        data: &[u8],
        access_list: &[(H160, Vec<H256>)],
        args: &Self::TransactArgs,
    ) -> Result<Self, ExitError> {
        let inner_state = State::new_transact_call(runtime, gas_limit, data, access_list, args)?;
        let jam_gas_limit = gas_limit.min(U256::from(u64::MAX)).as_u64();
        Ok(JAMGasState::new(inner_state, jam_gas_limit, true))
    }

    fn new_transact_create(
        runtime: evm::interpreter::runtime::RuntimeState,
        gas_limit: U256,
        code: &[u8],
        access_list: &[(H160, Vec<H256>)],
        args: &Self::TransactArgs,
    ) -> Result<Self, ExitError> {
        let inner_state = State::new_transact_create(runtime, gas_limit, code, access_list, args)?;
        let jam_gas_limit = gas_limit.min(U256::from(u64::MAX)).as_u64();
        Ok(JAMGasState::new(inner_state, jam_gas_limit, true))
    }

    fn substate(
        &mut self,
        runtime: evm::interpreter::runtime::RuntimeState,
        gas_limit: U256,
        is_static: bool,
        call_has_value: bool,
    ) -> Result<Self, ExitError> {
        let inner_substate = self
            .inner
            .substate(runtime, gas_limit, is_static, call_has_value)?;
        let jam_gas_limit = gas_limit.min(U256::from(u64::MAX)).as_u64();
        Ok(JAMGasState::new(
            inner_substate,
            jam_gas_limit,
            self.debug_enabled,
        ))
    }

    fn merge(&mut self, substate: Self, strategy: evm::MergeStrategy) {
        // Merge the inner states
        self.inner.merge(substate.inner, strategy);

        // For JAM gas, we don't need to merge gasometer state since we use host function
        // The JAM host function automatically accounts for all nested execution
        if self.debug_enabled {
            log_debug(&format!(
                "JAMGasState: merged substate with strategy {:?}",
                strategy
            ));
        }
    }

    fn record_codedeposit(&mut self, len: usize) -> Result<(), ExitError> {
        // Code deposit is a vendor gas concept - ignore for JAM gas model
        // The JAM host function will account for any real storage costs
        if self.debug_enabled {
            log_debug(&format!(
                "JAMGasState: ignoring codedeposit gas for {} bytes (zero-cost opcodes)",
                len
            ));
        }
        Ok(())
    }

    fn is_static(&self) -> bool {
        self.inner.is_static()
    }

    fn effective_gas(&self, _with_refund: bool) -> U256 {
        // Return remaining gas for proper invoker calculations
        // Invoker expects: used_gas = gas_limit - effective_gas
        let used = self.current_jam_gas_used();
        let remaining = self.jam_gas_limit.saturating_sub(used);
        U256::from(remaining)
    }

    fn reported_gas_used(&self) -> Option<U256> {
        Some(U256::from(self.current_jam_gas_used()))
    }
}
