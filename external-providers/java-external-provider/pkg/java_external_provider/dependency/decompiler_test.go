package dependency

import (
	"context"
	"io/fs"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/go-logr/logr/testr"
)

type testLabeler struct{}

func (t *testLabeler) HasLabel(string) bool {
	return false
}

func (t *testLabeler) AddLabels(_ string, _ bool) []string {
	return nil
}

var jarProjectOutput = map[string]any{
	"LICENSE": nil,
	"pom.xml": nil,
	"src/main/java/com/acmeair/entities/CustomerSession.java":            nil,
	"src/main/java/com/acmeair/entities/AirportCodeMapping.java":         nil,
	"src/main/java/com/acmeair/entities/Booking.java":                    nil,
	"src/main/java/com/acmeair/entities/BookingPK.java":                  nil,
	"src/main/java/com/acmeair/entities/CustomerAddress.java":            nil,
	"src/main/java/com/acmeair/entities/Customer.java":                   nil,
	"src/main/java/com/acmeair/entities/Flight.java":                     nil,
	"src/main/java/com/acmeair/entities/FlightPK.java":                   nil,
	"src/main/java/com/acmeair/entities/FlightSegment.java":              nil,
	"META-INF/MANIFEST.MF":                                               nil,
	"META-INF/maven/net.wasdev.wlp.sample/acmeair-common/pom.properties": nil,
}

var warProjectOutput = map[string]any{
	"favicon.ico":                                                  nil,
	"mileage.csv":                                                  nil,
	"src/main/webapp/css/style.css":                                nil,
	"src/main/webapp/images/AcmeAir.png":                           nil,
	"src/main/webapp/images/acmeAirplane.png":                      nil,
	"src/main/webapp/images/CloudBack.jpg":                         nil,
	"src/main/webapp/images/CloudBack2X.jpg":                       nil,
	"src/main/webapp/js/acmeair-common.js":                         nil,
	"src/main/webapp/WEB-INF/web.xml":                              nil,
	"src/main/webapp/checkin.html":                                 nil,
	"src/main/webapp/customerprofile.html":                         nil,
	"src/main/webapp/flights.html":                                 nil,
	"src/main/webapp/index.html":                                   nil,
	"src/main/java/LICENSE":                                        nil,
	"src/main/java/META-INF/persistence.xml":                       nil,
	"src/main/java/com/acmeair/web/BookingInfo.java":               nil,
	"src/main/java/com/acmeair/web/BookingsREST.java":              nil,
	"src/main/java/com/acmeair/web/CustomerREST.java":              nil,
	"src/main/java/com/acmeair/web/FlightsREST.java":               nil,
	"src/main/java/com/acmeair/web/LoaderREST.java":                nil,
	"src/main/java/com/acmeair/web/LoginREST.java":                 nil,
	"src/main/java/com/acmeair/web/RESTCookieSessionFilter.java":   nil,
	"src/main/java/com/acmeair/web/ServiceLocator.java":            nil,
	"src/main/java/com/acmeair/web/TripFlightOptions.java":         nil,
	"src/main/java/com/acmeair/web/TripLegInfo.java":               nil,
	"src/main/java/com/acmeair/web/config/WXSDirectAppConfig.java": nil,
}

var earProjectOutput = map[string]any{
	"pom.xml":               nil,
	"LogEventTopic-jms.xml": nil,
	"META-INF/MANIFEST.MF":  nil,
	"META-INF/ejb-jar.xml":  nil,
	"META-INF/maven/org.windup.example/jee-example-services/pom.properties": nil,
	"META-INF/maven/org.migration.support/migration-support/pom.properties": nil,
	"META-INF/weblogic-application.xml":                                     nil,
	"META-INF/weblogic-ejb-jar.xml":                                         nil,
	"META-INF/application.xml":                                              nil,
	"org/apache/log4j/lf5/config/defaultconfig.properties":                  nil,
	"org/apache/log4j/xml/log4j.dtd":                                        nil,
	"org/apache/log4j/lf5/viewer/images/channelexplorer_satellite.gif":      nil,
	"org/apache/log4j/lf5/viewer/images/channelexplorer_new.gif":            nil,
	"org/apache/log4j/lf5/viewer/images/lf5_small_icon.gif":                 nil,
	"src/main/java/org/apache/log4j/Appender.java":                          nil,
	"src/main/java/org/apache/log4j/AppenderSkeleton.java":                  nil,
	"src/main/java/org/apache/log4j/AsyncAppender.java":                     nil,
	"src/main/java/org/apache/log4j/BasicConfigurator.java":                 nil,
	"src/main/java/org/apache/log4j/Category.java":                          nil,
	"src/main/java/org/apache/log4j/CategoryKey.java":                       nil,
	"src/main/java/org/apache/log4j/ConsoleAppender.java":                   nil,
	"src/main/java/org/apache/log4j/DailyRollingFileAppender.java":          nil,
	"src/main/java/org/apache/log4j/DefaultCategoryFactory.java":            nil,
	"src/main/java/org/apache/log4j/Dispatcher.java":                        nil,
	"src/main/java/org/apache/log4j/FileAppender.java":                      nil,
	"src/main/java/org/apache/log4j/HTMLLayout.java":                        nil,
	"src/main/java/org/apache/log4j/Hierarchy.java":                         nil,
	"src/main/java/org/apache/log4j/Layout.java":                            nil,
	"src/main/java/org/apache/log4j/Level.java":                             nil,
	"src/main/java/org/apache/log4j/LogManager.java":                        nil,
	"src/main/java/org/apache/log4j/Logger.java":                            nil,
	"src/main/java/org/apache/log4j/MDC.java":                               nil,
	"src/main/java/org/apache/log4j/NDC.java":                               nil,
	"src/main/java/org/apache/log4j/PatternLayout.java":                     nil,
	"src/main/java/org/apache/log4j/Priority.java":                          nil,
	"src/main/java/org/apache/log4j/PropertyConfigurator.java":              nil,
	"src/main/java/org/apache/log4j/PropertyWatchdog.java":                  nil,
	"src/main/java/org/apache/log4j/ProvisionNode.java":                     nil,
	"src/main/java/org/apache/log4j/RollingCalendar.java":                   nil,
	"src/main/java/org/apache/log4j/RollingFileAppender.java":               nil,
	"src/main/java/org/apache/log4j/SimpleLayout.java":                      nil,
	"src/main/java/org/apache/log4j/TTCCLayout.java":                        nil,
	"src/main/java/org/apache/log4j/WriterAppender.java":                    nil,
	"src/main/java/org/apache/log4j/chainsaw/ControlPanel.java":             nil,
	"src/main/java/org/apache/log4j/chainsaw/DetailPanel.java":              nil,
	"src/main/java/org/apache/log4j/chainsaw/EventDetails.java":             nil,
	"src/main/java/org/apache/log4j/chainsaw/ExitAction.java":               nil,
	"src/main/java/org/apache/log4j/chainsaw/LoadXMLAction.java":            nil,
	"src/main/java/org/apache/log4j/chainsaw/LoggingReceiver.java":          nil,
	"src/main/java/org/apache/log4j/chainsaw/Main.java":                     nil,
	"src/main/java/org/apache/log4j/chainsaw/MyTableModel.java":             nil,
	"src/main/java/org/apache/log4j/chainsaw/XMLFileHandler.java":           nil,
	"src/main/java/org/apache/log4j/config/PropertyGetter.java":             nil,
	"src/main/java/org/apache/log4j/config/PropertyPrinter.java":            nil,
	"src/main/java/org/apache/log4j/config/PropertySetter.java":             nil,
	"src/main/java/org/apache/log4j/config/PropertySetterException.java":    nil,
	"src/main/java/org/apache/log4j/helpers/AbsoluteTimeDateFormat.java":    nil,
	"src/main/java/org/apache/log4j/helpers/AppenderAttachableImpl.java":    nil,
	"src/main/java/org/apache/log4j/helpers/BoundedFIFO.java":               nil,
	"src/main/java/org/apache/log4j/helpers/CountingQuietWriter.java":       nil,
	"src/main/java/org/apache/log4j/helpers/CyclicBuffer.java":              nil,
	"src/main/java/org/apache/log4j/helpers/DateLayout.java":                nil,
	"src/main/java/org/apache/log4j/helpers/DateTimeDateFormat.java":        nil,
	"src/main/java/org/apache/log4j/helpers/FileWatchdog.java":              nil,
	"src/main/java/org/apache/log4j/helpers/FormattingInfo.java":            nil,
	"src/main/java/org/apache/log4j/helpers/ISO8601DateFormat.java":         nil,
	"src/main/java/org/apache/log4j/helpers/Loader.java":                    nil,
	"src/main/java/org/apache/log4j/helpers/LogLog.java":                    nil,
	"src/main/java/org/apache/log4j/helpers/NullEnumeration.java":           nil,
	"src/main/java/org/apache/log4j/helpers/OnlyOnceErrorHandler.java":      nil,
	"src/main/java/org/apache/log4j/helpers/OptionConverter.java":           nil,
	"src/main/java/org/apache/log4j/helpers/PatternConverter.java":          nil,
	"src/main/java/org/apache/log4j/helpers/PatternParser.java":             nil,
	"src/main/java/org/apache/log4j/helpers/QuietWriter.java":               nil,
	"src/main/java/org/apache/log4j/helpers/RelativeTimeDateFormat.java":    nil,
	"src/main/java/org/apache/log4j/helpers/SyslogQuietWriter.java":         nil,
	"src/main/java/org/apache/log4j/helpers/SyslogWriter.java":              nil,
	"src/main/java/org/apache/log4j/helpers/ThreadLocalMap.java":            nil,
	"src/main/java/org/apache/log4j/helpers/Transform.java":                 nil,
	"src/main/java/org/apache/log4j/jdbc/JDBCAppender.java":                 nil,
	"src/main/java/org/apache/log4j/jmx/AbstractDynamicMBean.java":          nil,
	"src/main/java/org/apache/log4j/jmx/Agent.java":                         nil,
	"src/main/java/org/apache/log4j/jmx/AppenderDynamicMBean.java":          nil,
	"src/main/java/org/apache/log4j/jmx/HierarchyDynamicMBean.java":         nil,
	"src/main/java/org/apache/log4j/jmx/LayoutDynamicMBean.java":            nil,
	"src/main/java/org/apache/log4j/jmx/LoggerDynamicMBean.java":            nil,
	"src/main/java/org/apache/log4j/jmx/MethodUnion.java":                   nil,
	"src/main/java/org/apache/log4j/lf5/AppenderFinalizer.java":             nil,
	//"src/main/java/org/apache/log4j/lf5/DefaultLF5Appender.java":                                      nil,
	"src/main/java/org/apache/log4j/lf5/DefaultLF5Configurator.java":                                  nil,
	"src/main/java/org/apache/log4j/lf5/LF5Appender.java":                                             nil,
	"src/main/java/org/apache/log4j/lf5/Log4JLogRecord.java":                                          nil,
	"src/main/java/org/apache/log4j/lf5/LogLevel.java":                                                nil,
	"src/main/java/org/apache/log4j/lf5/LogLevelFormatException.java":                                 nil,
	"src/main/java/org/apache/log4j/lf5/LogRecord.java":                                               nil,
	"src/main/java/org/apache/log4j/lf5/LogRecordFilter.java":                                         nil,
	"src/main/java/org/apache/log4j/lf5/PassingLogRecordFilter.java":                                  nil,
	"src/main/java/org/apache/log4j/lf5/StartLogFactor5.java":                                         nil,
	"src/main/java/org/apache/log4j/lf5/config/defaultconfig.properties":                              nil,
	"src/main/java/org/apache/log4j/lf5/util/AdapterLogRecord.java":                                   nil,
	"src/main/java/org/apache/log4j/lf5/util/DateFormatManager.java":                                  nil,
	"src/main/java/org/apache/log4j/lf5/util/LogFileParser.java":                                      nil,
	"src/main/java/org/apache/log4j/lf5/util/LogMonitorAdapter.java":                                  nil,
	"src/main/java/org/apache/log4j/lf5/util/Resource.java":                                           nil,
	"src/main/java/org/apache/log4j/lf5/util/ResourceUtils.java":                                      nil,
	"src/main/java/org/apache/log4j/lf5/util/StreamUtils.java":                                        nil,
	"src/main/java/org/apache/log4j/lf5/viewer/FilteredLogTableModel.java":                            nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogBrokerMonitor.java":                                 nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogFactor5Dialog.java":                                 nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogFactor5ErrorDialog.java":                            nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogFactor5InputDialog.java":                            nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogFactor5LoadingDialog.java":                          nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogTable.java":                                         nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogTableColumn.java":                                   nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogTableColumnFormatException.java":                    nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogTableModel.java":                                    nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LogTableRowRenderer.java":                              nil,
	"src/main/java/org/apache/log4j/lf5/viewer/TrackingAdjustmentListener.java":                       nil,
	"src/main/java/org/apache/log4j/lf5/viewer/LF5SwingUtils.java":                                    nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryAbstractCellEditor.java":      nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryElement.java":                 nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryExplorerLogRecordFilter.java": nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryExplorerModel.java":           nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryExplorerTree.java":            nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryImmediateEditor.java":         nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryNode.java":                    nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryNodeEditor.java":              nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryNodeEditorRenderer.java":      nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryNodeRenderer.java":            nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/CategoryPath.java":                    nil,
	"src/main/java/org/apache/log4j/lf5/viewer/categoryexplorer/TreeModelAdapter.java":                nil,
	"src/main/java/org/apache/log4j/lf5/viewer/configure/ConfigurationManager.java":                   nil,
	"src/main/java/org/apache/log4j/lf5/viewer/configure/MRUFileManager.java":                         nil,
	"src/main/java/org/apache/log4j/lf5/viewer/images/channelexplorer_new.gif":                        nil,
	"src/main/java/org/apache/log4j/lf5/viewer/images/channelexplorer_satellite.gif":                  nil,
	"src/main/java/org/apache/log4j/lf5/viewer/images/lf5_small_icon.gif":                             nil,
	"src/main/java/org/apache/log4j/net/DefaultEvaluator.java":                                        nil,
	"src/main/java/org/apache/log4j/net/JMSAppender.java":                                             nil,
	"src/main/java/org/apache/log4j/net/JMSSink.java":                                                 nil,
	"src/main/java/org/apache/log4j/net/SMTPAppender.java":                                            nil,
	"src/main/java/org/apache/log4j/net/SocketAppender.java":                                          nil,
	"src/main/java/org/apache/log4j/net/SimpleSocketServer.java":                                      nil,
	"src/main/java/org/apache/log4j/net/SocketHubAppender.java":                                       nil,
	"src/main/java/org/apache/log4j/net/SocketNode.java":                                              nil,
	"src/main/java/org/apache/log4j/net/SocketServer.java":                                            nil,
	"src/main/java/org/apache/log4j/net/SyslogAppender.java":                                          nil,
	"src/main/java/org/apache/log4j/net/TelnetAppender.java":                                          nil,
	"src/main/java/org/apache/log4j/nt/NTEventLogAppender.java":                                       nil,
	"src/main/java/org/apache/log4j/or/DefaultRenderer.java":                                          nil,
	"src/main/java/org/apache/log4j/or/ObjectRenderer.java":                                           nil,
	"src/main/java/org/apache/log4j/or/RendererMap.java":                                              nil,
	"src/main/java/org/apache/log4j/or/ThreadGroupRenderer.java":                                      nil,
	"src/main/java/org/apache/log4j/or/sax/AttributesRenderer.java":                                   nil,
	"src/main/java/org/apache/log4j/spi/AppenderAttachable.java":                                      nil,
	"src/main/java/org/apache/log4j/spi/Configurator.java":                                            nil,
	"src/main/java/org/apache/log4j/spi/DefaultRepositorySelector.java":                               nil,
	"src/main/java/org/apache/log4j/spi/ErrorCode.java":                                               nil,
	"src/main/java/org/apache/log4j/spi/ErrorHandler.java":                                            nil,
	"src/main/java/org/apache/log4j/spi/Filter.java":                                                  nil,
	"src/main/java/org/apache/log4j/spi/HierarchyEventListener.java":                                  nil,
	"src/main/java/org/apache/log4j/spi/LocationInfo.java":                                            nil,
	"src/main/java/org/apache/log4j/spi/LoggerFactory.java":                                           nil,
	"src/main/java/org/apache/log4j/spi/LoggerRepository.java":                                        nil,
	"src/main/java/org/apache/log4j/spi/LoggingEvent.java":                                            nil,
	"src/main/java/org/apache/log4j/spi/NullWriter.java":                                              nil,
	"src/main/java/org/apache/log4j/spi/OptionHandler.java":                                           nil,
	"src/main/java/org/apache/log4j/spi/RendererSupport.java":                                         nil,
	"src/main/java/org/apache/log4j/spi/RepositorySelector.java":                                      nil,
	"src/main/java/org/apache/log4j/spi/RootCategory.java":                                            nil,
	"src/main/java/org/apache/log4j/spi/ThrowableInformation.java":                                    nil,
	"src/main/java/org/apache/log4j/spi/TriggeringEventEvaluator.java":                                nil,
	"src/main/java/org/apache/log4j/spi/VectorWriter.java":                                            nil,
	"src/main/java/org/apache/log4j/varia/DenyAllFilter.java":                                         nil,
	"src/main/java/org/apache/log4j/varia/ExternallyRolledFileAppender.java":                          nil,
	"src/main/java/org/apache/log4j/varia/FallbackErrorHandler.java":                                  nil,
	"src/main/java/org/apache/log4j/varia/HUP.java":                                                   nil,
	"src/main/java/org/apache/log4j/varia/HUPNode.java":                                               nil,
	"src/main/java/org/apache/log4j/varia/LevelMatchFilter.java":                                      nil,
	"src/main/java/org/apache/log4j/varia/LevelRangeFilter.java":                                      nil,
	"src/main/java/org/apache/log4j/varia/NullAppender.java":                                          nil,
	"src/main/java/org/apache/log4j/varia/ReloadingPropertyConfigurator.java":                         nil,
	"src/main/java/org/apache/log4j/varia/Roller.java":                                                nil,
	"src/main/java/org/apache/log4j/varia/StringMatchFilter.java":                                     nil,
	"src/main/java/org/apache/log4j/xml/DOMConfigurator.java":                                         nil,
	"src/main/java/org/apache/log4j/xml/SAXErrorHandler.java":                                         nil,
	"src/main/java/org/apache/log4j/xml/XMLLayout.java":                                               nil,
	"src/main/java/org/apache/log4j/xml/XMLWatchdog.java":                                             nil,
	"src/main/java/org/apache/log4j/xml/log4j.dtd":                                                    nil,
	"src/main/java/weblogic/application/ApplicationContext.java":                                      nil,
	"src/main/java/weblogic/application/ApplicationLifecycleListener.java":                            nil,
	"src/main/java/weblogic/common/T3ServicesDef.java":                                                nil,
	"src/main/java/weblogic/common/T3StartupDef.java":                                                 nil,
	"src/main/java/weblogic/ejb/GenericMessageDrivenBean.java":                                        nil,
	"src/main/java/weblogic/ejb/GenericSessionBean.java":                                              nil,
	"src/main/java/weblogic/ejbgen/ActivationConfigProperty.java":                                     nil,
	"src/main/java/weblogic/ejbgen/MessageDriven.java":                                                nil,
	"src/main/java/weblogic/i18n/logging/NonCatalogLogger.java":                                       nil,
	"src/main/java/weblogic/jndi/Environment.java":                                                    nil,
	"src/main/java/weblogic/logging/log4j/Log4jLoggingHelper.java":                                    nil,
	"src/main/java/weblogic/logging/LoggerNotAvailableException.java":                                 nil,
	"src/main/java/weblogic/management/MBeanHome.java":                                                nil,
	"src/main/java/weblogic/application/ApplicationLifecycleEvent.java":                               nil,
	"src/main/java/weblogic/security/acl/UserInfo.java":                                               nil,
	"src/main/java/weblogic/security/services/AppContext.java":                                        nil,
	"src/main/java/weblogic/security/services/AppContextElement.java":                                 nil,
	"src/main/java/weblogic/servlet/security/ServletAuthentication.java":                              nil,
	"src/main/java/weblogic/transaction/ClientTransactionManager.java":                                nil,
	"src/main/java/weblogic/transaction/ClientTxHelper.java":                                          nil,
	"src/main/java/weblogic/transaction/InterposedTransactionManager.java":                            nil,
	"src/main/java/weblogic/transaction/nonxa/NonXAResource.java":                                     nil,
	"src/main/java/weblogic/transaction/Transaction.java":                                             nil,
	"src/main/java/weblogic/transaction/TransactionHelper.java":                                       nil,
	"src/main/java/weblogic/transaction/TransactionManager.java":                                      nil,
	"src/main/java/weblogic/transaction/TxHelper.java":                                                nil,
	"src/main/java/weblogic/transaction/UserTransaction.java":                                         nil,
	"src/main/java/weblogic/transaction/XAResource.java":                                              nil,
	"src/main/java/org/migration/support/NotImplemented.java":                                         nil,
	"src/main/java/com/acme/anvil/listener/AnvilWebLifecycleListener.java":                            nil,
	"src/main/java/com/acme/anvil/listener/AnvilWebStartupListener.java":                              nil,
	"src/main/java/com/acme/anvil/management/AnvilInvokeBean.java":                                    nil,
	"src/main/java/com/acme/anvil/management/AnvilInvokeBeanImpl.java":                                nil,
	"src/main/java/com/acme/anvil/service/ItemLookup.java":                                            nil,
	"src/main/java/com/acme/anvil/service/ItemLookupBean.java":                                        nil,
	"src/main/java/com/acme/anvil/service/ItemLookupHome.java":                                        nil,
	"src/main/java/com/acme/anvil/service/ItemLookupLocal.java":                                       nil,
	"src/main/java/com/acme/anvil/service/ItemLookupLocalHome.java":                                   nil,
	"src/main/java/com/acme/anvil/service/jms/LogEventPublisher.java":                                 nil,
	"src/main/java/com/acme/anvil/service/jms/LogEventSubscriber.java":                                nil,
	"src/main/java/com/acme/anvil/service/ProductCatalog.java":                                        nil,
	"src/main/java/com/acme/anvil/service/ProductCatalogBean.java":                                    nil,
	"src/main/java/com/acme/anvil/service/ProductCatalogHome.java":                                    nil,
	"src/main/java/com/acme/anvil/service/ProductCatalogLocal.java":                                   nil,
	"src/main/java/com/acme/anvil/service/ProductCatalogLocalHome.java":                               nil,
	"src/main/java/com/acme/anvil/vo/Item.java":                                                       nil,
	"src/main/java/com/acme/anvil/vo/LogEvent.java":                                                   nil,
	"src/main/java/com/acme/anvil/AnvilWebServlet.java":                                               nil,
	"src/main/java/com/acme/anvil/AuthenticateFilter.java":                                            nil,
	"src/main/java/com/acme/anvil/LoginFilter.java":                                                   nil,
	"src/main/webapp/WEB-INF/faces-config.xml":                                                        nil,
	"src/main/webapp/WEB-INF/web.xml":                                                                 nil,
	"src/main/webapp/WEB-INF/weblogic.xml":                                                            nil,
}

type testProject struct {
	output map[string]any
}

func (p testProject) matchProject(dir string, t *testing.T) {
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fail()
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := p.output[filepath.ToSlash(relPath)]; !ok {
			t.Logf("could not find file: %v", filepath.ToSlash(relPath))
			t.Fail()
		} else {
			p.output[filepath.ToSlash(relPath)] = &struct{}{}
		}

		return nil
	})
}

func (p testProject) foundAllFiles() []string {
	missed := []string{}
	for str, val := range p.output {
		if val == nil {
			missed = append(missed, str)
		}
	}
	return missed
}

type testMavenDir struct {
	output map[string]any
}

func (m testMavenDir) matchMavenDir(dir string, t *testing.T) {
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		relPath, err := filepath.Rel(dir, path)
		if err != nil {
			t.Fail()
			return err
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := m.output[filepath.ToSlash(relPath)]; !ok {
			t.Logf("relPath: %v", filepath.ToSlash(relPath))
			t.Logf("could not find file: %v", path)
			t.Fail()
		} else {
			m.output[filepath.ToSlash(relPath)] = &struct{}{}
		}

		return nil
	})
}

func (m testMavenDir) foundAllFiles() []string {
	missed := []string{}
	for str, val := range m.output {
		if val == nil {
			missed = append(missed, str)
		}
	}
	return missed
}

var jarProjectMavenDir = map[string]any{}
var warProjectMavenDir = map[string]any{
	"io/konveyor/embededdep/commons-logging-1.1.1/0.0.0-SNAPSHOT/commons-logging-1.1.1-0.0.0-SNAPSHOT.jar":                                 nil,
	"io/konveyor/embededdep/commons-logging-1.1.1/0.0.0-SNAPSHOT/commons-logging-1.1.1-0.0.0-SNAPSHOT-sources.jar":                         nil,
	"io/konveyor/embededdep/aopalliance-1.0/0.0.0-SNAPSHOT/aopalliance-1.0-0.0.0-SNAPSHOT.jar":                                             nil,
	"io/konveyor/embededdep/aopalliance-1.0/0.0.0-SNAPSHOT/aopalliance-1.0-0.0.0-SNAPSHOT-sources.jar":                                     nil,
	"io/konveyor/embededdep/asm-3.3.1/0.0.0-SNAPSHOT/asm-3.3.1-0.0.0-SNAPSHOT.jar":                                                         nil,
	"io/konveyor/embededdep/asm-3.3.1/0.0.0-SNAPSHOT/asm-3.3.1-0.0.0-SNAPSHOT-sources.jar":                                                 nil,
	"io/konveyor/embededdep/aspectjrt-1.6.8/0.0.0-SNAPSHOT/aspectjrt-1.6.8-0.0.0-SNAPSHOT.jar":                                             nil,
	"io/konveyor/embededdep/aspectjrt-1.6.8/0.0.0-SNAPSHOT/aspectjrt-1.6.8-0.0.0-SNAPSHOT-sources.jar":                                     nil,
	"io/konveyor/embededdep/aspectjweaver-1.6.8/0.0.0-SNAPSHOT/aspectjweaver-1.6.8-0.0.0-SNAPSHOT.jar":                                     nil,
	"io/konveyor/embededdep/aspectjweaver-1.6.8/0.0.0-SNAPSHOT/aspectjweaver-1.6.8-0.0.0-SNAPSHOT-sources.jar":                             nil,
	"io/konveyor/embededdep/cglib-2.2.2/0.0.0-SNAPSHOT/cglib-2.2.2-0.0.0-SNAPSHOT.jar":                                                     nil,
	"io/konveyor/embededdep/cglib-2.2.2/0.0.0-SNAPSHOT/cglib-2.2.2-0.0.0-SNAPSHOT-sources.jar":                                             nil,
	"io/konveyor/embededdep/spring-aop-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-aop-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                           nil,
	"io/konveyor/embededdep/spring-aop-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-aop-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":                   nil,
	"io/konveyor/embededdep/spring-asm-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-asm-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                           nil,
	"io/konveyor/embededdep/spring-asm-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-asm-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":                   nil,
	"io/konveyor/embededdep/spring-beans-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-beans-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                       nil,
	"io/konveyor/embededdep/spring-beans-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-beans-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":               nil,
	"io/konveyor/embededdep/spring-context-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-context-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                   nil,
	"io/konveyor/embededdep/spring-context-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-context-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":           nil,
	"io/konveyor/embededdep/spring-core-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-core-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                         nil,
	"io/konveyor/embededdep/spring-core-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-core-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":                 nil,
	"io/konveyor/embededdep/spring-expression-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-expression-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":             nil,
	"io/konveyor/embededdep/spring-expression-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-expression-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":     nil,
	"io/konveyor/embededdep/spring-tx-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-tx-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                             nil,
	"io/konveyor/embededdep/spring-tx-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-tx-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":                     nil,
	"io/konveyor/embededdep/spring-web-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-web-3.1.2.RELEASE-0.0.0-SNAPSHOT.jar":                           nil,
	"io/konveyor/embededdep/spring-web-3.1.2.RELEASE/0.0.0-SNAPSHOT/spring-web-3.1.2.RELEASE-0.0.0-SNAPSHOT-sources.jar":                   nil,
	"io/konveyor/embededdep/acmeair-common-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-common-1.0-SNAPSHOT-0.0.0-SNAPSHOT.jar":                     nil,
	"io/konveyor/embededdep/acmeair-common-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-common-1.0-SNAPSHOT-0.0.0-SNAPSHOT-sources.jar":             nil,
	"io/konveyor/embededdep/acmeair-services-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-services-1.0-SNAPSHOT-0.0.0-SNAPSHOT.jar":                 nil,
	"io/konveyor/embededdep/acmeair-services-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-services-1.0-SNAPSHOT-0.0.0-SNAPSHOT-sources.jar":         nil,
	"io/konveyor/embededdep/acmeair-services-jpa-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-services-jpa-1.0-SNAPSHOT-0.0.0-SNAPSHOT.jar":         nil,
	"io/konveyor/embededdep/acmeair-services-jpa-1.0-SNAPSHOT/0.0.0-SNAPSHOT/acmeair-services-jpa-1.0-SNAPSHOT-0.0.0-SNAPSHOT-sources.jar": nil,
}
var earProjectMavenDir = map[string]any{
	"io/konveyor/embededdep/migration-support-1.0.0/0.0.0-SNAPSHOT/migration-support-1.0.0-0.0.0-SNAPSHOT.jar":         nil,
	"io/konveyor/embededdep/migration-support-1.0.0/0.0.0-SNAPSHOT/migration-support-1.0.0-0.0.0-SNAPSHOT-sources.jar": nil,
	"io/konveyor/embededdep/log4j-1.2.6/0.0.0-SNAPSHOT/log4j-1.2.6-0.0.0-SNAPSHOT.jar":                                 nil,
	"io/konveyor/embededdep/log4j-1.2.6/0.0.0-SNAPSHOT/log4j-1.2.6-0.0.0-SNAPSHOT-sources.jar":                         nil,
	"io/konveyor/embededdep/commons-lang-2.5/0.0.0-SNAPSHOT/commons-lang-2.5-0.0.0-SNAPSHOT.jar":                       nil,
	"io/konveyor/embededdep/commons-lang-2.5/0.0.0-SNAPSHOT/commons-lang-2.5-0.0.0-SNAPSHOT-sources.jar":               nil,
}

func TestDecompile(t *testing.T) {
	testCases := []struct {
		Name        string
		archivePath string
		testProject testProject
		mavenDir    testMavenDir
		artifacts   []JavaArtifact
	}{
		{
			Name:        "Decompile_Common_Jar",
			archivePath: "testdata/acmeair-common-1.0-SNAPSHOT.jar",
			testProject: testProject{output: jarProjectOutput},
			mavenDir:    testMavenDir{output: jarProjectMavenDir},
			artifacts:   []JavaArtifact{},
		},
		{
			Name:        "Decompile_War",
			archivePath: "testdata/acmeair-webapp-1.0-SNAPSHOT.war",
			testProject: testProject{output: warProjectOutput},
			mavenDir:    testMavenDir{output: warProjectMavenDir},
			artifacts: []JavaArtifact{
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "commons-logging",
					ArtifactId:  "commons-loging",
					Version:     "1.1.1",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "aopalliance",
					Version:     "1.0",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "asm-3.3.1",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "aspectjrt-1.6.8",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "aspectjweaver-1.6.8",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "cglib-2.2.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-aop-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-aop-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-asm-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-beans-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-context-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-core-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-expression-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-tx-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: false,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "spring-web-3.1.2",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "net/wasdeb.wlp.sample",
					ArtifactId:  "acmeair-common",
					Version:     "1.0-SNAPSHOT",
				},
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "net/wasdeb.wlp.sample",
					ArtifactId:  "acmeair-services",
					Version:     "1.0-SNAPSHOT",
				},
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "net/wasdeb.wlp.sample",
					ArtifactId:  "acmeair-services-jpa",
					Version:     "1.0-SNAPSHOT",
				},
			},
		},
		{
			Name:        "Decompile_Ear",
			archivePath: "testdata/jee-example-app-1.0.0.ear",
			testProject: testProject{output: earProjectOutput},
			mavenDir:    testMavenDir{output: earProjectMavenDir},
			artifacts: []JavaArtifact{
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "org.migration.support",
					ArtifactId:  "migration-support",
					Version:     "1.1.0",
				},
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "io.konveyor.embededdep",
					ArtifactId:  "log4j-1.2.6",
					Version:     "0.0.0-SNAPSHOT",
				},
				{
					FoundOnline: true,
					Packaging:   ".jar",
					GroupId:     "commons-lang",
					ArtifactId:  "commons-lang",
					Version:     "2.5",
				},
			},
		},
	}

	for _, test := range testCases {
		fernflower, err := filepath.Abs("testdata/fernflower.jar")
		// Defer cleanup only if the test passes
		if err != nil {
			t.Fail()
		}
		t.Run(test.Name, func(t *testing.T) {
			// Need to get the Decompiler.
			mavenDir := t.TempDir()
			projectTmpDir := t.TempDir()

			decompiler, err := getDecompiler(DecompilerOpts{
				DecompileTool: fernflower,
				log: testr.NewWithOptions(t, testr.Options{
					Verbosity: 20,
				}),
				workers:        10,
				labler:         &testLabeler{},
				mavenIndexPath: "test",
				m2Repo:         filepath.Clean(mavenDir),
			})
			if err != nil {
				t.Fail()
			}

			p, err := filepath.Abs(test.archivePath)
			if err != nil {
				t.Fail()
			}
			artifacts, err := decompiler.DecompileIntoProject(context.Background(), p, filepath.Clean(projectTmpDir))
			if err != nil {
				t.Fail()
			}
			test.testProject.matchProject(projectTmpDir, t)
			missed := test.testProject.foundAllFiles()
			if len(missed) > 0 {
				t.Logf("missed: %#v", missed)
				t.Fail()
			}
			test.mavenDir.matchMavenDir(mavenDir, t)
			missed = test.mavenDir.foundAllFiles()
			if len(missed) > 0 {
				t.Logf("missed: %#v", missed)
				t.Fail()
			}
			if len(test.artifacts) != len(artifacts) && reflect.DeepEqual(test.artifacts, artifacts) {
				t.Logf("Artifacts Not Equal:\nexpected: %v\nactual: %v", test.artifacts, artifacts)
				t.Fail()
			}
		})
	}
}
