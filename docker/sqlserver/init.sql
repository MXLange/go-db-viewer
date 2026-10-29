USE [master];
GO

IF DB_ID(N'dbviewer') IS NULL
    EXEC(N'CREATE DATABASE [dbviewer]');
GO

IF NOT EXISTS (SELECT 1 FROM sys.server_principals WHERE name = N'dbviewer')
    CREATE LOGIN [dbviewer] WITH PASSWORD = N'LocalDev2026', CHECK_POLICY = ON, CHECK_EXPIRATION = OFF;
GO

USE [dbviewer];
GO

IF NOT EXISTS (SELECT 1 FROM sys.database_principals WHERE name = N'dbviewer')
    CREATE USER [dbviewer] FOR LOGIN [dbviewer] WITH DEFAULT_SCHEMA = [dbo];
GO

IF IS_ROLEMEMBER(N'db_owner', N'dbviewer') <> 1
    ALTER ROLE [db_owner] ADD MEMBER [dbviewer];
GO

PRINT N'Banco dbviewer e usuário dbviewer disponíveis.';
GO
